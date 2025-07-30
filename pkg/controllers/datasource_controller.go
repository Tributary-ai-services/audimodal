package controllers

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	audimodalv1 "github.com/jscharber/eAIIngest/api/v1"
)

// DataSourceReconciler reconciles a DataSource object
type DataSourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	tracer trace.Tracer
}

//+kubebuilder:rbac:groups=audimodal.ai,resources=datasources,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=audimodal.ai,resources=datasources/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=audimodal.ai,resources=datasources/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs;cronjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps;secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile implements the main reconciliation logic for DataSource resources
func (r *DataSourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, span := r.tracer.Start(ctx, "datasource_reconcile")
	defer span.End()

	logger := log.FromContext(ctx)
	
	span.SetAttributes(
		attribute.String("datasource.name", req.Name),
		attribute.String("datasource.namespace", req.Namespace),
	)

	// Fetch the DataSource instance
	var dataSource audimodalv1.DataSource
	if err := r.Get(ctx, req.NamespacedName, &dataSource); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("DataSource resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		span.RecordError(err)
		logger.Error(err, "Failed to get DataSource")
		return ctrl.Result{}, err
	}

	// Handle finalizer for cleanup
	finalizerName := "datasource.audimodal.ai/finalizer"
	if dataSource.ObjectMeta.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(&dataSource, finalizerName) {
			controllerutil.AddFinalizer(&dataSource, finalizerName)
			return ctrl.Result{}, r.Update(ctx, &dataSource)
		}
	} else {
		// Handle deletion
		if controllerutil.ContainsFinalizer(&dataSource, finalizerName) {
			if err := r.cleanupDataSource(ctx, &dataSource); err != nil {
				span.RecordError(err)
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&dataSource, finalizerName)
			return ctrl.Result{}, r.Update(ctx, &dataSource)
		}
		return ctrl.Result{}, nil
	}

	// Set initial status if needed
	if dataSource.Status.Phase == "" {
		dataSource.Status.Phase = "Pending"
		dataSource.Status.Conditions = []audimodalv1.DataSourceCondition{
			{
				Type:               "Ready",
				Status:             "False",
				LastTransitionTime: metav1.Now(),
				Reason:             "Initializing",
				Message:            "DataSource is being initialized",
			},
		}
		if err := r.Status().Update(ctx, &dataSource); err != nil {
			span.RecordError(err)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip reconciliation if disabled
	if !dataSource.Spec.Enabled {
		r.updateDataSourceStatus(ctx, &dataSource, "Suspended", "DataSource is disabled")
		return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
	}

	// Reconcile data source resources
	result, err := r.reconcileDataSourceResources(ctx, &dataSource)
	if err != nil {
		span.RecordError(err)
		r.updateDataSourceStatus(ctx, &dataSource, "Error", err.Error())
		return result, err
	}

	// Validate credentials and connectivity
	if err := r.validateDataSourceConnection(ctx, &dataSource); err != nil {
		span.RecordError(err)
		r.updateDataSourceStatus(ctx, &dataSource, "Error", fmt.Sprintf("Connection validation failed: %v", err))
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// Schedule sync if configured
	if err := r.scheduleSyncJob(ctx, &dataSource); err != nil {
		span.RecordError(err)
		r.updateDataSourceStatus(ctx, &dataSource, "Error", fmt.Sprintf("Failed to schedule sync: %v", err))
		return ctrl.Result{RequeueAfter: time.Minute * 2}, nil
	}

	// Update status to active
	r.updateDataSourceStatus(ctx, &dataSource, "Active", "DataSource is active and ready for processing")

	span.SetAttributes(
		attribute.String("datasource.phase", dataSource.Status.Phase),
		attribute.String("datasource.type", dataSource.Spec.Type),
	)

	// Requeue based on sync mode
	requeueAfter := time.Minute * 5
	if dataSource.Spec.Sync.Mode == "realtime" {
		requeueAfter = time.Minute * 1
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// reconcileDataSourceResources creates and manages all resources for a data source
func (r *DataSourceReconciler) reconcileDataSourceResources(ctx context.Context, dataSource *audimodalv1.DataSource) (ctrl.Result, error) {
	ctx, span := r.tracer.Start(ctx, "reconcile_datasource_resources")
	defer span.End()

	// Create or update configuration ConfigMap
	if err := r.createDataSourceConfig(ctx, dataSource); err != nil {
		span.RecordError(err)
		return ctrl.Result{}, err
	}

	// Validate credentials if specified
	if dataSource.Spec.Config.Credentials.SecretRef.Name != "" {
		if err := r.validateCredentials(ctx, dataSource); err != nil {
			span.RecordError(err)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// createDataSourceConfig creates a ConfigMap with data source configuration
func (r *DataSourceReconciler) createDataSourceConfig(ctx context.Context, dataSource *audimodalv1.DataSource) error {
	configData := map[string]string{
		"name":        dataSource.Spec.Name,
		"type":        dataSource.Spec.Type,
		"enabled":     fmt.Sprintf("%t", dataSource.Spec.Enabled),
		"priority":    dataSource.Spec.Priority,
		"sync_mode":   dataSource.Spec.Sync.Mode,
		"schedule":    dataSource.Spec.Sync.Schedule,
		"batch_size":  fmt.Sprintf("%d", dataSource.Spec.Sync.BatchSize),
		"max_file_size": dataSource.Spec.Config.MaxFileSize,
	}

	// Add type-specific configuration
	if dataSource.Spec.Config.Path != "" {
		configData["path"] = dataSource.Spec.Config.Path
	}
	if dataSource.Spec.Config.Bucket != "" {
		configData["bucket"] = dataSource.Spec.Config.Bucket
	}
	if dataSource.Spec.Config.Region != "" {
		configData["region"] = dataSource.Spec.Config.Region
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("datasource-%s", dataSource.Name),
			Namespace: dataSource.Namespace,
			Labels: map[string]string{
				"app":                         "audimodal",
				"audimodal.ai/datasource":     dataSource.Name,
				"audimodal.ai/datasource-type": dataSource.Spec.Type,
			},
		},
		Data: configData,
	}

	if err := controllerutil.SetControllerReference(dataSource, configMap, r.Scheme); err != nil {
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

// validateCredentials validates that the referenced secret exists and has required fields
func (r *DataSourceReconciler) validateCredentials(ctx context.Context, dataSource *audimodalv1.DataSource) error {
	secretRef := dataSource.Spec.Config.Credentials.SecretRef
	secret := &corev1.Secret{}
	
	secretNamespace := secretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = dataSource.Namespace
	}

	if err := r.Get(ctx, types.NamespacedName{Name: secretRef.Name, Namespace: secretNamespace}, secret); err != nil {
		return fmt.Errorf("failed to get credentials secret: %w", err)
	}

	// Validate required fields based on data source type
	switch dataSource.Spec.Type {
	case "s3":
		if _, ok := secret.Data["access_key_id"]; !ok {
			return fmt.Errorf("missing access_key_id in credentials secret")
		}
		if _, ok := secret.Data["secret_access_key"]; !ok {
			return fmt.Errorf("missing secret_access_key in credentials secret")
		}
	case "sharepoint":
		if _, ok := secret.Data["client_id"]; !ok {
			return fmt.Errorf("missing client_id in credentials secret")
		}
		if _, ok := secret.Data["client_secret"]; !ok {
			return fmt.Errorf("missing client_secret in credentials secret")
		}
	case "database":
		if _, ok := secret.Data["connection_string"]; !ok {
			return fmt.Errorf("missing connection_string in credentials secret")
		}
	}

	return nil
}

// validateDataSourceConnection performs a connection test to the data source
func (r *DataSourceReconciler) validateDataSourceConnection(ctx context.Context, dataSource *audimodalv1.DataSource) error {
	// This would implement actual connectivity testing
	// For now, we'll simulate validation based on type
	switch dataSource.Spec.Type {
	case "filesystem":
		// Validate path exists and is readable
		if dataSource.Spec.Config.Path == "" {
			return fmt.Errorf("path is required for filesystem data source")
		}
	case "s3", "gcs", "azure_blob":
		// Validate cloud storage credentials and bucket access
		if dataSource.Spec.Config.Bucket == "" {
			return fmt.Errorf("bucket is required for cloud storage data source")
		}
	case "sharepoint", "google_drive", "box", "dropbox", "onedrive":
		// Validate API credentials and permissions
		if dataSource.Spec.Config.Credentials.SecretRef.Name == "" {
			return fmt.Errorf("credentials are required for %s data source", dataSource.Spec.Type)
		}
	case "database":
		// Validate database connection
		if dataSource.Spec.Config.ConnectionString == "" && dataSource.Spec.Config.Credentials.SecretRef.Name == "" {
			return fmt.Errorf("connection string or credentials are required for database data source")
		}
	case "api":
		// Validate API endpoint
		if dataSource.Spec.Config.Endpoint == "" {
			return fmt.Errorf("endpoint is required for API data source")
		}
	}

	return nil
}

// scheduleSyncJob creates or updates sync jobs based on the sync configuration
func (r *DataSourceReconciler) scheduleSyncJob(ctx context.Context, dataSource *audimodalv1.DataSource) error {
	switch dataSource.Spec.Sync.Mode {
	case "scheduled":
		return r.createCronJob(ctx, dataSource)
	case "manual":
		// No automatic scheduling needed
		return nil
	case "realtime", "webhook":
		// These modes require different handling (e.g., webhooks, event listeners)
		return r.setupRealtimeSync(ctx, dataSource)
	default:
		return fmt.Errorf("unsupported sync mode: %s", dataSource.Spec.Sync.Mode)
	}
}

// createCronJob creates a CronJob for scheduled data source synchronization
func (r *DataSourceReconciler) createCronJob(ctx context.Context, dataSource *audimodalv1.DataSource) error {
	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("datasource-sync-%s", dataSource.Name),
			Namespace: dataSource.Namespace,
			Labels: map[string]string{
				"app":                         "audimodal",
				"audimodal.ai/datasource":     dataSource.Name,
				"audimodal.ai/job-type":       "sync",
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule: dataSource.Spec.Sync.Schedule,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									Name:  "sync",
									Image: "audimodal/sync:latest", // This would be the actual sync image
									Env: []corev1.EnvVar{
										{
											Name:  "DATASOURCE_NAME",
											Value: dataSource.Name,
										},
										{
											Name:  "DATASOURCE_TYPE",
											Value: dataSource.Spec.Type,
										},
										{
											Name:  "BATCH_SIZE",
											Value: fmt.Sprintf("%d", dataSource.Spec.Sync.BatchSize),
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "config",
											MountPath: "/etc/datasource",
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "config",
									VolumeSource: corev1.VolumeSource{
										ConfigMap: &corev1.ConfigMapVolumeSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: fmt.Sprintf("datasource-%s", dataSource.Name),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(dataSource, cronJob, r.Scheme); err != nil {
		return err
	}

	existing := &batchv1.CronJob{}
	if err := r.Get(ctx, types.NamespacedName{Name: cronJob.Name, Namespace: cronJob.Namespace}, existing); err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, cronJob)
		}
		return err
	}

	// Update schedule if changed
	if existing.Spec.Schedule != cronJob.Spec.Schedule {
		existing.Spec.Schedule = cronJob.Spec.Schedule
		return r.Update(ctx, existing)
	}

	return nil
}

// setupRealtimeSync sets up real-time synchronization for the data source
func (r *DataSourceReconciler) setupRealtimeSync(ctx context.Context, dataSource *audimodalv1.DataSource) error {
	// This would set up webhooks, event listeners, or streaming connections
	// Implementation depends on the specific data source type and requirements
	return nil
}

// updateDataSourceStatus updates the data source status
func (r *DataSourceReconciler) updateDataSourceStatus(ctx context.Context, dataSource *audimodalv1.DataSource, phase, message string) {
	dataSource.Status.Phase = phase
	
	// Update conditions
	now := metav1.Now()
	conditionType := "Ready"
	conditionStatus := "True"
	reason := "DataSourceReady"
	
	if phase != "Active" {
		conditionStatus = "False"
		reason = "DataSourceNotReady"
		if phase == "Error" {
			reason = "DataSourceError"
		}
	}

	// Find existing condition or create new one
	conditionUpdated := false
	for i, condition := range dataSource.Status.Conditions {
		if condition.Type == conditionType {
			dataSource.Status.Conditions[i].Status = conditionStatus
			dataSource.Status.Conditions[i].LastTransitionTime = now
			dataSource.Status.Conditions[i].Reason = reason
			dataSource.Status.Conditions[i].Message = message
			conditionUpdated = true
			break
		}
	}

	if !conditionUpdated {
		dataSource.Status.Conditions = append(dataSource.Status.Conditions, audimodalv1.DataSourceCondition{
			Type:               conditionType,
			Status:             conditionStatus,
			LastTransitionTime: now,
			Reason:             reason,
			Message:            message,
		})
	}

	// Update health status
	dataSource.Status.Health.Status = "healthy"
	dataSource.Status.Health.LastCheck = now
	dataSource.Status.Health.Message = message
	if phase == "Error" {
		dataSource.Status.Health.Status = "unhealthy"
	}

	dataSource.Status.ObservedGeneration = dataSource.Generation
	r.Status().Update(ctx, dataSource)
}

// cleanupDataSource cleans up data source resources when deleted
func (r *DataSourceReconciler) cleanupDataSource(ctx context.Context, dataSource *audimodalv1.DataSource) error {
	ctx, span := r.tracer.Start(ctx, "cleanup_datasource")
	defer span.End()

	// Delete CronJob if it exists
	cronJob := &batchv1.CronJob{}
	cronJobName := fmt.Sprintf("datasource-sync-%s", dataSource.Name)
	if err := r.Get(ctx, types.NamespacedName{Name: cronJobName, Namespace: dataSource.Namespace}, cronJob); err == nil {
		if err := r.Delete(ctx, cronJob); err != nil && !errors.IsNotFound(err) {
			span.RecordError(err)
			return err
		}
	}

	// Delete ConfigMap if it exists
	configMap := &corev1.ConfigMap{}
	configMapName := fmt.Sprintf("datasource-%s", dataSource.Name)
	if err := r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: dataSource.Namespace}, configMap); err == nil {
		if err := r.Delete(ctx, configMap); err != nil && !errors.IsNotFound(err) {
			span.RecordError(err)
			return err
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *DataSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.tracer = otel.Tracer("datasource-controller")
	
	return ctrl.NewControllerManagedBy(mgr).
		For(&audimodalv1.DataSource{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&batchv1.CronJob{}).
		Complete(r)
}