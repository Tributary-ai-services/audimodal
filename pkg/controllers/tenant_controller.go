//go:build ignore
// +build ignore

//nolint:all // Kubernetes controller - client.Client methods undefined due to structured-merge-diff dependency conflict
package controllers

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	audimodalv1 "github.com/jscharber/eAIIngest/api/v1"
)

// TenantReconciler reconciles a Tenant object
type TenantReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	tracer trace.Tracer
}

//+kubebuilder:rbac:groups=audimodal.ai,resources=tenants,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=audimodal.ai,resources=tenants/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=audimodal.ai,resources=tenants/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=namespaces;serviceaccounts;configmaps;secrets;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

// Reconcile implements the main reconciliation logic for Tenant resources
func (r *TenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, span := r.tracer.Start(ctx, "tenant_reconcile")
	defer span.End()

	logger := log.FromContext(ctx)

	span.SetAttributes(
		attribute.String("tenant.name", req.Name),
		attribute.String("tenant.namespace", req.Namespace),
	)

	// Fetch the Tenant instance
	var tenant audimodalv1.Tenant
	if err := r.Get(ctx, req.NamespacedName, &tenant); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Tenant resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		span.RecordError(err)
		logger.Error(err, "Failed to get Tenant")
		return ctrl.Result{}, err
	}

	// Handle finalizer for cleanup
	finalizerName := "tenant.audimodal.ai/finalizer"
	if tenant.ObjectMeta.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(&tenant, finalizerName) {
			controllerutil.AddFinalizer(&tenant, finalizerName)
			return ctrl.Result{}, r.Update(ctx, &tenant)
		}
	} else {
		// Handle deletion
		if controllerutil.ContainsFinalizer(&tenant, finalizerName) {
			if err := r.cleanupTenant(ctx, &tenant); err != nil {
				span.RecordError(err)
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&tenant, finalizerName)
			return ctrl.Result{}, r.Update(ctx, &tenant)
		}
		return ctrl.Result{}, nil
	}

	// Set initial status if needed
	if tenant.Status.Phase == "" {
		tenant.Status.Phase = "Pending"
		tenant.Status.Conditions = []audimodalv1.TenantCondition{
			{
				Type:               "Ready",
				Status:             "False",
				LastTransitionTime: metav1.Now(),
				Reason:             "Initializing",
				Message:            "Tenant is being initialized",
			},
		}
		if err := r.Status().Update(ctx, &tenant); err != nil {
			span.RecordError(err)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Reconcile tenant resources
	result, err := r.reconcileTenantResources(ctx, &tenant)
	if err != nil {
		span.RecordError(err)
		r.updateTenantStatus(ctx, &tenant, "Failed", err.Error())
		return result, err
	}

	// Update status to active if all resources are ready
	if r.areResourcesReady(ctx, &tenant) {
		r.updateTenantStatus(ctx, &tenant, "Active", "All tenant resources are ready")
	} else {
		r.updateTenantStatus(ctx, &tenant, "Provisioning", "Provisioning tenant resources")
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	span.SetAttributes(
		attribute.String("tenant.phase", tenant.Status.Phase),
		attribute.Int("tenant.conditions", len(tenant.Status.Conditions)),
	)

	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

// reconcileTenantResources creates and manages all resources for a tenant
func (r *TenantReconciler) reconcileTenantResources(ctx context.Context, tenant *audimodalv1.Tenant) (ctrl.Result, error) {
	ctx, span := r.tracer.Start(ctx, "reconcile_tenant_resources")
	defer span.End()

	// Create tenant namespace
	if err := r.createTenantNamespace(ctx, tenant); err != nil {
		span.RecordError(err)
		return ctrl.Result{}, err
	}

	// Create service account
	if err := r.createTenantServiceAccount(ctx, tenant); err != nil {
		span.RecordError(err)
		return ctrl.Result{}, err
	}

	// Create RBAC resources
	if err := r.createTenantRBAC(ctx, tenant); err != nil {
		span.RecordError(err)
		return ctrl.Result{}, err
	}

	// Create network policies if enabled
	if tenant.Spec.Security.NetworkPolicies {
		if err := r.createNetworkPolicies(ctx, tenant); err != nil {
			span.RecordError(err)
			return ctrl.Result{}, err
		}
	}

	// Create storage resources
	if err := r.createTenantStorage(ctx, tenant); err != nil {
		span.RecordError(err)
		return ctrl.Result{}, err
	}

	// Create configuration
	if err := r.createTenantConfig(ctx, tenant); err != nil {
		span.RecordError(err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// createTenantNamespace creates a dedicated namespace for the tenant
func (r *TenantReconciler) createTenantNamespace(ctx context.Context, tenant *audimodalv1.Tenant) error {
	namespaceName := fmt.Sprintf("audimodal-%s", tenant.Spec.Slug)
	if namespaceName == "" {
		namespaceName = fmt.Sprintf("audimodal-%s", tenant.Name)
	}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
			Labels: map[string]string{
				"app":                          "audimodal",
				"audimodal.ai/tenant":          tenant.Name,
				"audimodal.ai/tenant-plan":     tenant.Spec.Plan,
				"audimodal.ai/isolation-level": tenant.Spec.Security.IsolationLevel,
			},
			Annotations: map[string]string{
				"audimodal.ai/tenant-id":  string(tenant.UID),
				"audimodal.ai/created-by": "audimodal-operator",
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(tenant, namespace, r.Scheme); err != nil {
		return err
	}

	// Create or update namespace
	existing := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Name: namespaceName}, existing); err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, namespace)
		}
		return err
	}

	// Update namespace labels if needed
	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	for k, v := range namespace.Labels {
		existing.Labels[k] = v
	}

	return r.Update(ctx, existing)
}

// createTenantServiceAccount creates a service account for tenant workloads
func (r *TenantReconciler) createTenantServiceAccount(ctx context.Context, tenant *audimodalv1.Tenant) error {
	namespaceName := fmt.Sprintf("audimodal-%s", tenant.Spec.Slug)
	if namespaceName == "" {
		namespaceName = fmt.Sprintf("audimodal-%s", tenant.Name)
	}

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "audimodal-tenant",
			Namespace: namespaceName,
			Labels: map[string]string{
				"app":                 "audimodal",
				"audimodal.ai/tenant": tenant.Name,
			},
		},
	}

	if err := controllerutil.SetControllerReference(tenant, serviceAccount, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.ServiceAccount{}
	if err := r.Get(ctx, types.NamespacedName{Name: serviceAccount.Name, Namespace: serviceAccount.Namespace}, existing); err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, serviceAccount)
		}
		return err
	}

	return nil
}

// createTenantRBAC creates RBAC resources for the tenant
func (r *TenantReconciler) createTenantRBAC(ctx context.Context, tenant *audimodalv1.Tenant) error {
	namespaceName := fmt.Sprintf("audimodal-%s", tenant.Spec.Slug)
	if namespaceName == "" {
		namespaceName = fmt.Sprintf("audimodal-%s", tenant.Name)
	}

	// Create Role
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "audimodal-tenant-role",
			Namespace: namespaceName,
			Labels: map[string]string{
				"app":                 "audimodal",
				"audimodal.ai/tenant": tenant.Name,
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps", "secrets", "persistentvolumeclaims"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch"},
			},
			{
				APIGroups: []string{"audimodal.ai"},
				Resources: []string{"datasources", "processingsessions", "dlppolicies"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
		},
	}

	if err := controllerutil.SetControllerReference(tenant, role, r.Scheme); err != nil {
		return err
	}

	existingRole := &rbacv1.Role{}
	if err := r.Get(ctx, types.NamespacedName{Name: role.Name, Namespace: role.Namespace}, existingRole); err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, role); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	// Create RoleBinding
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "audimodal-tenant-binding",
			Namespace: namespaceName,
			Labels: map[string]string{
				"app":                 "audimodal",
				"audimodal.ai/tenant": tenant.Name,
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "audimodal-tenant",
				Namespace: namespaceName,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     "audimodal-tenant-role",
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	if err := controllerutil.SetControllerReference(tenant, roleBinding, r.Scheme); err != nil {
		return err
	}

	existingBinding := &rbacv1.RoleBinding{}
	if err := r.Get(ctx, types.NamespacedName{Name: roleBinding.Name, Namespace: roleBinding.Namespace}, existingBinding); err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, roleBinding)
		}
		return err
	}

	return nil
}

// createNetworkPolicies creates network policies for tenant isolation
func (r *TenantReconciler) createNetworkPolicies(ctx context.Context, tenant *audimodalv1.Tenant) error {
	// Implementation would create NetworkPolicy resources
	// This is a placeholder for the actual network policy creation
	return nil
}

// createTenantStorage creates storage resources for the tenant
func (r *TenantReconciler) createTenantStorage(ctx context.Context, tenant *audimodalv1.Tenant) error {
	namespaceName := fmt.Sprintf("audimodal-%s", tenant.Spec.Slug)
	if namespaceName == "" {
		namespaceName = fmt.Sprintf("audimodal-%s", tenant.Name)
	}

	// Create PVC for tenant data
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-data",
			Namespace: namespaceName,
			Labels: map[string]string{
				"app":                 "audimodal",
				"audimodal.ai/tenant": tenant.Name,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(tenant.Spec.Resources.Storage),
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(tenant, pvc, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, existing); err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, pvc)
		}
		return err
	}

	return nil
}

// createTenantConfig creates configuration resources for the tenant
func (r *TenantReconciler) createTenantConfig(ctx context.Context, tenant *audimodalv1.Tenant) error {
	namespaceName := fmt.Sprintf("audimodal-%s", tenant.Spec.Slug)
	if namespaceName == "" {
		namespaceName = fmt.Sprintf("audimodal-%s", tenant.Name)
	}

	configData := map[string]string{
		"tenant_id":                 string(tenant.UID),
		"tenant_name":               tenant.Spec.Name,
		"tenant_plan":               tenant.Spec.Plan,
		"max_storage_gb":            fmt.Sprintf("%d", tenant.Spec.Settings.MaxStorageGB),
		"max_users":                 fmt.Sprintf("%d", tenant.Spec.Settings.MaxUsers),
		"max_data_sources":          fmt.Sprintf("%d", tenant.Spec.Settings.MaxDataSources),
		"retention_days":            fmt.Sprintf("%d", tenant.Spec.Settings.RetentionDays),
		"enable_dlp":                fmt.Sprintf("%t", tenant.Spec.Settings.EnableDLP),
		"enable_advanced_analytics": fmt.Sprintf("%t", tenant.Spec.Settings.EnableAdvancedAnalytics),
		"encryption_at_rest":        fmt.Sprintf("%t", tenant.Spec.Security.EncryptionAtRest),
		"encryption_in_transit":     fmt.Sprintf("%t", tenant.Spec.Security.EncryptionInTransit),
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-config",
			Namespace: namespaceName,
			Labels: map[string]string{
				"app":                 "audimodal",
				"audimodal.ai/tenant": tenant.Name,
			},
		},
		Data: configData,
	}

	if err := controllerutil.SetControllerReference(tenant, configMap, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, existing); err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, configMap)
		}
		return err
	}

	// Update if data changed
	existing.Data = configData
	return r.Update(ctx, existing)
}

// areResourcesReady checks if all tenant resources are ready
func (r *TenantReconciler) areResourcesReady(ctx context.Context, tenant *audimodalv1.Tenant) bool {
	namespaceName := fmt.Sprintf("audimodal-%s", tenant.Spec.Slug)
	if namespaceName == "" {
		namespaceName = fmt.Sprintf("audimodal-%s", tenant.Name)
	}

	// Check namespace
	namespace := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Name: namespaceName}, namespace); err != nil {
		return false
	}

	// Check service account
	serviceAccount := &corev1.ServiceAccount{}
	if err := r.Get(ctx, types.NamespacedName{Name: "audimodal-tenant", Namespace: namespaceName}, serviceAccount); err != nil {
		return false
	}

	// Check PVC
	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, types.NamespacedName{Name: "tenant-data", Namespace: namespaceName}, pvc); err != nil {
		return false
	}

	return pvc.Status.Phase == corev1.ClaimBound
}

// updateTenantStatus updates the tenant status
func (r *TenantReconciler) updateTenantStatus(ctx context.Context, tenant *audimodalv1.Tenant, phase, message string) {
	tenant.Status.Phase = phase

	// Update conditions
	now := metav1.Now()
	conditionType := "Ready"
	conditionStatus := "True"
	reason := "ResourcesReady"

	if phase != "Active" {
		conditionStatus = "False"
		reason = "ResourcesNotReady"
	}

	// Find existing condition or create new one
	conditionUpdated := false
	for i, condition := range tenant.Status.Conditions {
		if condition.Type == conditionType {
			tenant.Status.Conditions[i].Status = conditionStatus
			tenant.Status.Conditions[i].LastTransitionTime = now
			tenant.Status.Conditions[i].Reason = reason
			tenant.Status.Conditions[i].Message = message
			conditionUpdated = true
			break
		}
	}

	if !conditionUpdated {
		tenant.Status.Conditions = append(tenant.Status.Conditions, audimodalv1.TenantCondition{
			Type:               conditionType,
			Status:             conditionStatus,
			LastTransitionTime: now,
			Reason:             reason,
			Message:            message,
		})
	}

	tenant.Status.ObservedGeneration = tenant.Generation
	r.Status().Update(ctx, tenant)
}

// cleanupTenant cleans up tenant resources when the tenant is deleted
func (r *TenantReconciler) cleanupTenant(ctx context.Context, tenant *audimodalv1.Tenant) error {
	ctx, span := r.tracer.Start(ctx, "cleanup_tenant")
	defer span.End()

	namespaceName := fmt.Sprintf("audimodal-%s", tenant.Spec.Slug)
	if namespaceName == "" {
		namespaceName = fmt.Sprintf("audimodal-%s", tenant.Name)
	}

	// Delete the tenant namespace (this will cascade delete all resources in it)
	namespace := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Name: namespaceName}, namespace); err == nil {
		if err := r.Delete(ctx, namespace); err != nil && !errors.IsNotFound(err) {
			span.RecordError(err)
			return err
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.tracer = otel.Tracer("tenant-controller")

	return ctrl.NewControllerManagedBy(mgr).
		For(&audimodalv1.Tenant{}).
		Owns(&corev1.Namespace{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Complete(r)
}
