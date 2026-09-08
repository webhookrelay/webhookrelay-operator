package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/webhookrelay/webhookrelay-operator/pkg/apis"
	"github.com/webhookrelay/webhookrelay-operator/pkg/controller"
	"github.com/webhookrelay/webhookrelay-operator/version"
)

const (
	metricsAddress = "0.0.0.0:8383"
	healthAddress  = "0.0.0.0:8986"
	leaderLockID   = "webhookrelay-operator-lock"
)

var setupLog = ctrl.Log.WithName("setup")

func main() {
	loggerOptions := zap.Options{}
	loggerOptions.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&loggerOptions)))

	printVersion()

	watchNamespaces, err := configuredNamespaces(os.Getenv("WATCH_NAMESPACE"))
	if err != nil {
		setupLog.Error(err, "invalid watch namespace configuration")
		os.Exit(1)
	}

	options := ctrl.Options{
		Cache: cache.Options{DefaultNamespaces: watchNamespaces},
		Metrics: metricsserver.Options{
			BindAddress: metricsAddress,
		},
		HealthProbeBindAddress:        healthAddress,
		LeaderElection:                true,
		LeaderElectionID:              leaderLockID,
		LeaderElectionResourceLock:    resourcelock.LeasesResourceLock,
		LeaderElectionReleaseOnCancel: true,
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to register API scheme")
		os.Exit(1)
	}
	if err := controller.AddToManager(mgr); err != nil {
		setupLog.Error(err, "unable to register controllers")
		os.Exit(1)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to register health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to register readiness check")
		os.Exit(1)
	}

	setupLog.Info("starting manager", "watchNamespaces", namespaceNames(watchNamespaces))
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "manager exited non-zero")
		os.Exit(1)
	}
}

func configuredNamespaces(value string) (map[string]cache.Config, error) {
	result := make(map[string]cache.Config)
	for _, namespace := range strings.Split(value, ",") {
		namespace = strings.TrimSpace(namespace)
		if namespace == "" {
			continue
		}
		if problems := validation.IsDNS1123Label(namespace); len(problems) > 0 {
			return nil, fmt.Errorf("namespace %q is invalid: %s", namespace, strings.Join(problems, "; "))
		}
		result[namespace] = cache.Config{}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("WATCH_NAMESPACE must contain at least one namespace")
	}
	return result, nil
}

func namespaceNames(namespaces map[string]cache.Config) []string {
	result := make([]string, 0, len(namespaces))
	for namespace := range namespaces {
		result = append(result, namespace)
	}
	sort.Strings(result)
	return result
}

func printVersion() {
	buildInfo := version.GetBuildInfo()
	setupLog.Info("build information",
		"version", buildInfo.Version,
		"buildDate", buildInfo.BuildDate,
		"revision", buildInfo.Revision,
		"goVersion", runtime.Version(),
		"platform", runtime.GOOS+"/"+runtime.GOARCH,
		"apiVersion", "forward.webhookrelay.com/v1",
	)
}
