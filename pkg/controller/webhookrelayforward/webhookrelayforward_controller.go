package webhookrelayforward

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
	"github.com/webhookrelay/webhookrelay-operator/pkg/config"
)

var log = logf.Log.WithName("controller_webhookrelayforward")

const (
	reconcilePeriodSeconds = 5

	// containerTokenKeyEnvName and containerTokenSecretEnvName used
	// to specify authentication details for the container
	containerTokenKeyEnvName    = "KEY"
	containerTokenSecretEnvName = "SECRET"
	// containerBucketsEnvName specify which buckets the agent should
	// subscribe to
	containerBucketsEnvName = "BUCKETS"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new WebhookRelayForward Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	cfg := config.MustLoad()
	return &ReconcileWebhookRelayForward{
		client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor("webhookrelay-forwarder"),
		config:   &cfg,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("webhookrelayforward-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource WebhookRelayForward
	err = c.Watch(&source.Kind{Type: &forwardv1.WebhookRelayForward{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Deployments and requeue the owner WebhookRelayForward
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &forwardv1.WebhookRelayForward{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileWebhookRelayForward implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileWebhookRelayForward{}

// ReconcileWebhookRelayForward reconciles a WebhookRelayForward object
type ReconcileWebhookRelayForward struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client   client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder

	apiClient *WebhookRelayClient
	config    *config.Config
}

// Reconcile reads that state of the cluster for a WebhookRelayForward object and makes changes based on the state read
// and what is in the WebhookRelayForward.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileWebhookRelayForward) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	// logger.Info("reconciling")

	reconcilePeriod := reconcilePeriodSeconds * time.Second
	reconcileResult := reconcile.Result{RequeueAfter: reconcilePeriod}
	reconcileImmediately := reconcile.Result{RequeueAfter: time.Second}

	// Fetch the WebhookRelayForward instance
	instance := &forwardv1.WebhookRelayForward{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcileResult, err
	}

	// Compare the instance names, generations and UIDs to check if it's
	// the same instance. Update the client if client instance name,
	// generation or UID are different from current instance. In theory,
	// CRs can be used by different Webhook Relay accounts so we shouldn't
	// reuse the same client
	if r.apiClient == nil ||
		r.apiClient.instanceName != instance.GetName() ||
		r.apiClient.instanceGeneration != instance.GetGeneration() ||
		r.apiClient.instanceUID != instance.GetUID() {
		if err := r.setClientForCluster(instance); err != nil {
			logger.Error(err, "Failed to configure Webhook Relay API client, cannot continue")
			return reconcileResult, err
		}
		logger.Info("API client initialized")
	}

	if err := r.ensureRoutingConfiguration(logger, instance); err != nil {
		logger.Error(err, "encountered errors while ensuring routing configuration, check your CR spec")
		// If configuration fails, we still need to ensure deployment is running, however
		// we still need to report it
		requeue, updateErr := r.updateRoutingStatus(
			logger,
			forwardv1.RoutingStatusFailed,
			fmt.Sprintf("encountered errors (%s) while ensuring routing configuration, check your CR spec", err),
			instance,
		)
		if updateErr != nil {

			if !strings.Contains(updateErr.Error(), "Operation cannot be fulfille") {
				logger.Error(updateErr, "Failed to update CR routing configuration status",
					"status", forwardv1.AgentStatusCreating,
				)
			}
		}
		if requeue {
			logger.Info("routing status updated, requeuing")
			return reconcileImmediately, updateErr
		}

	} else {
		// Setting status to Configured
		requeue, updateErr := r.updateRoutingStatus(
			logger,
			forwardv1.RoutingStatusConfigured,
			"",
			instance,
		)
		if updateErr != nil {
			logger.Error(updateErr, "Failed to update CR status")
		}
		if requeue {
			logger.Info("routing status updated, requeuing")
			return reconcileImmediately, updateErr
		}
	}

	if err := r.reconcile(logger, instance); err != nil {
		logger.Info("Reconcile failed", "error", err)
	}
	return reconcileResult, nil
}

func (r *ReconcileWebhookRelayForward) updateRoutingStatus(logger logr.Logger, status forwardv1.RoutingStatus, message string, instance *forwardv1.WebhookRelayForward) (bool, error) {
	if instance.Status.RoutingStatus == status && instance.Status.Message == message {
		return false, nil
	}

	instance.Status.RoutingStatus = status
	instance.Status.Message = message

	logger.Info("Updating routing status",
		"phase", status,
		"message", message,
	)

	err := r.client.Status().Update(context.TODO(), instance)
	return true, err
}

func (r *ReconcileWebhookRelayForward) updateDeploymentStatus(logger logr.Logger, status forwardv1.AgentStatus, ready bool, instance *forwardv1.WebhookRelayForward) (bool, error) {
	if instance.Status.AgentStatus == status && instance.Status.Ready == ready {
		return false, nil
	}
	instance.Status.AgentStatus = status
	instance.Status.Ready = ready

	logger.Info("Updating deployment status",
		"status", status,
		"ready", ready,
	)

	err := r.client.Status().Update(context.TODO(), instance)
	return true, err
}

func (r *ReconcileWebhookRelayForward) reconcile(logger logr.Logger, instance *forwardv1.WebhookRelayForward) error {

	// Define a new Deployment object
	deployment := r.newDeploymentForCR(instance)

	// Set WebhookRelayForward instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, deployment, r.scheme); err != nil {
		return err
	}

	// Check if this Deployment already exists
	found := &appsv1.Deployment{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating a new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		err = r.client.Create(context.TODO(), deployment)
		if err != nil {
			r.recorder.Event(instance, corev1.EventTypeWarning, "FailedCreation", err.Error())

			_, updateErr := r.updateDeploymentStatus(logger, forwardv1.AgentStatusCreating, false, instance)
			if updateErr != nil {
				if !strings.Contains(updateErr.Error(), "Operation cannot be fulfille") {
					logger.Error(updateErr, "Failed to update CR status",
						"status", forwardv1.AgentStatusCreating,
					)
				}
			}
			return err
		}

		_, updateErr := r.updateDeploymentStatus(logger, forwardv1.AgentStatusRunning, true, instance)
		if updateErr != nil {
			if !strings.Contains(updateErr.Error(), "Operation cannot be fulfille") {
				logger.Error(updateErr, "Failed to update CR status",
					"status", forwardv1.AgentStatusRunning,
				)
			}
		}

		// Deployment created successfully - don't requeue
		return nil
	} else if err != nil {
		return err
	}

	// compare image, buckets
	patched, equals := r.checkDeployment(instance, found)
	if equals {
		// TODO: check replicas 1/1 for Ready status
		_, updateErr := r.updateDeploymentStatus(logger, forwardv1.AgentStatusRunning, true, instance)
		if updateErr != nil {
			if !strings.Contains(updateErr.Error(), "Operation cannot be fulfille") {
				logger.Error(updateErr, "Failed to update CR status",
					"status", forwardv1.AgentStatusRunning,
				)
			}
		}

		// Deployment already exists - don't requeue
		return nil
	}

	err = r.client.Update(context.TODO(), patched)
	if err != nil {
		r.recorder.Event(instance, corev1.EventTypeWarning, "FailedUpdate", err.Error())
		return fmt.Errorf("failed to update Deployment: %s", err)
	}

	logger.Info("Deployment updated")

	return nil
}
