package main

import (
	"flag"
	"os"

	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	cmgrv1 "github.com/kloudlite/cluster-operator/apis/cmgr/v1"
	infrav1 "github.com/kloudlite/cluster-operator/apis/infra/v1"
	cmgrcontrollers "github.com/kloudlite/cluster-operator/controllers/cmgr"
	infracontrollers "github.com/kloudlite/cluster-operator/controllers/infra"
	"github.com/kloudlite/cluster-operator/env"
	"github.com/kloudlite/cluster-operator/lib/logging"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(cmgrv1.AddToScheme(scheme))
	utilruntime.Must(infrav1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	var isDev bool
	flag.BoolVar(&isDev, "dev", false, "Enable development mode")

	opts := zap.Options{
		Development: true,
	}

	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	envVars := env.GetEnvOrDie()

	var mgr manager.Manager

	if isDev {
		mr, err := ctrl.NewManager(
			&rest.Config{
				Host: "localhost:8080",
			},
			ctrl.Options{
				Scheme:                     scheme,
				MetricsBindAddress:         metricsAddr,
				Port:                       9443,
				HealthProbeBindAddress:     probeAddr,
				LeaderElection:             enableLeaderElection,
				LeaderElectionID:           "cluster-operator.kloudlite.io",
				LeaderElectionResourceLock: "configmaps",
			},
		)
		if err != nil {
			setupLog.Error(err, "unable to start manager")
			os.Exit(1)
		}
		mgr = mr
	} else {

		mr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:                     scheme,
			MetricsBindAddress:         metricsAddr,
			Port:                       9443,
			HealthProbeBindAddress:     probeAddr,
			LeaderElection:             enableLeaderElection,
			LeaderElectionID:           "cluster-operator.kloudlite.io",
			LeaderElectionResourceLock: "configmaps",
		})

		if err != nil {
			setupLog.Error(err, "unable to start manager")
			os.Exit(1)
		}
		mgr = mr
	}

	logger := logging.NewOrDie(&logging.Options{Dev: true})

	if err := (&cmgrcontrollers.ClusterReconciler{
		Env:  envVars,
		Name: "Cluster",
	}).SetupWithManager(mgr, logger); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Cluster")
		os.Exit(1)
	}

	if err := (&cmgrcontrollers.MasterNodeReconciler{
		Env:  envVars,
		Name: "MasterNode",
	}).SetupWithManager(mgr, logger); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MasterNode")
		os.Exit(1)
	}

	if err := (&infracontrollers.CloudProviderReconciler{
		Name: "CLProvider",
	}).SetupWithManager(mgr, logger); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CloudProvider")
		os.Exit(1)
	}

	if err := (&infracontrollers.NodePoolReconciler{
		Name: "Nodepool",
	}).SetupWithManager(mgr, logger); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NodePool")
		os.Exit(1)
	}

	if err := (&infracontrollers.WorkerNodeReconciler{
		Env:  envVars,
		Name: "WorkerNode",
	}).SetupWithManager(mgr, logger); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "WorkerNode")
		os.Exit(1)
	}

	if err := (&infracontrollers.EdgeReconciler{
		Name: "Edge",
	}).SetupWithManager(mgr, logger); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AccountProvider")
		os.Exit(1)
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
