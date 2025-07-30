package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	// Import all Kubernetes client schemes
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	// Import AudiModal APIs and controllers
	audimodalv1 "github.com/jscharber/eAIIngest/api/v1"
	"github.com/jscharber/eAIIngest/pkg/controllers"
	"github.com/jscharber/eAIIngest/pkg/tracing"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(audimodalv1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var webhookPort int
	var logLevel string
	var enableTracing bool
	var tracingEndpoint string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.IntVar(&webhookPort, "webhook-port", 9443, "The port that the webhook server serves at.")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	flag.BoolVar(&enableTracing, "enable-tracing", true, "Enable OpenTelemetry tracing")
	flag.StringVar(&tracingEndpoint, "tracing-endpoint", "http://localhost:14268/api/traces", "OpenTelemetry tracing endpoint")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Set up logger
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog.Info("Starting AudiModal Kubernetes Operator",
		"version", "v1.0.0",
		"metrics-addr", metricsAddr,
		"probe-addr", probeAddr,
		"leader-election", enableLeaderElection,
		"webhook-port", webhookPort,
		"log-level", logLevel,
		"tracing-enabled", enableTracing,
	)

	// Initialize tracing if enabled
	var tracingService *tracing.TracingService
	if enableTracing {
		tracingConfig := &tracing.TracingConfig{
			ServiceName:     "audimodal-operator",
			ServiceVersion:  "1.0.0",
			Environment:     getEnv("ENVIRONMENT", "development"),
			Enabled:         true,
			SampleRate:      1.0,
			ExportType:      "jaeger",
			ExportEndpoint:  tracingEndpoint,
			ExportTimeout:   30 * time.Second,
		}

		var err error
		tracingService, err = tracing.NewTracingService(tracingConfig)
		if err != nil {
			setupLog.Error(err, "Failed to initialize tracing service")
			os.Exit(1)
		}

		if err := tracingService.Start(context.Background()); err != nil {
			setupLog.Error(err, "Failed to start tracing service")
			os.Exit(1)
		}
		defer tracingService.Stop(context.Background())

		setupLog.Info("Tracing service initialized successfully")
	}

	// Set up manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: webhookPort,
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "audimodal-operator-leader-election",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		LeaderElectionReleaseOnCancel: true,
		Cache: cache.Options{
			// Watch only our namespace and system namespaces for better performance
			DefaultNamespaces: map[string]cache.Config{
				"audimodal-system": {},
				"default":          {},
				"kube-system":      {},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	// Set up controllers
	if err = (&controllers.TenantReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "Tenant")
		os.Exit(1)
	}

	if err = (&controllers.DataSourceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "DataSource")
		os.Exit(1)
	}

	// Add health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up health check")
		os.Exit(1)
	}

	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up ready check")
		os.Exit(1)
	}

	// Set up webhooks if enabled (placeholder for future webhook implementation)
	// if os.Getenv("ENABLE_WEBHOOKS") != "false" {
	//     if err = (&audimodalv1.Tenant{}).SetupWebhookWithManager(mgr); err != nil {
	//         setupLog.Error(err, "unable to create webhook", "webhook", "Tenant")
	//         os.Exit(1)
	//     }
	// }

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Problem running manager")
		os.Exit(1)
	}
}

// getEnv gets an environment variable with a fallback default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}