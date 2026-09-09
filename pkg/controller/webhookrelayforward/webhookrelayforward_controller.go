package webhookrelayforward

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

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
	containerWebsocketEnvName   = "WEBSOCKET_TRANSPORT"
	// containerBucketsEnvName specify which buckets the agent should
	// subscribe to
	containerBucketsEnvName        = "BUCKETS"
	forwarderLabelKey              = "name"
	forwarderLabelValue            = "webhookrelay-forwarder"
	reasonDeploymentAvailable      = "DeploymentAvailable"
	reasonProgressDeadlineExceeded = "ProgressDeadlineExceeded"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new WebhookRelayForward Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&forwardv1.WebhookRelayForward{}).
		Owns(&appsv1.Deployment{}).
		Complete(newReconciler(mgr))
}

// newReconciler returns a new controller reconciler.
func newReconciler(mgr manager.Manager) *ReconcileWebhookRelayForward {
	cfg := config.MustLoad()
	return &ReconcileWebhookRelayForward{
		client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorder("webhookrelay-forwarder"),
		config:   &cfg,
	}
}

// ReconcileWebhookRelayForward reconciles a WebhookRelayForward object
type ReconcileWebhookRelayForward struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client   client.Client
	scheme   *runtime.Scheme
	recorder events.EventRecorder

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
func (r *ReconcileWebhookRelayForward) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	reconcilePeriod := reconcilePeriodSeconds * time.Second
	reconcileResult := ctrl.Result{RequeueAfter: reconcilePeriod}
	reconcileImmediately := ctrl.Result{RequeueAfter: time.Second}

	// Fetch the WebhookRelayForward instance
	instance := &forwardv1.WebhookRelayForward{}
	err := r.client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
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
		if err := r.setClientForCluster(ctx, instance); err != nil {
			logger.Error(err, "Failed to configure Webhook Relay API client, cannot continue")
			_, statusErr := r.updateRoutingStatus(logger, ctx, forwardv1.RoutingStatusFailed,
				"Relay API client configuration failed", instance)
			if statusErr != nil {
				return reconcileResult, statusErr
			}
			return reconcileResult, err
		}
		logger.Info("API client initialized")
	}

	if err := r.ensureRoutingConfiguration(ctx, logger, instance); err != nil {
		logger.Error(err, "encountered errors while ensuring routing configuration, check your CR spec")
		// If configuration fails, we still need to ensure deployment is running, however
		// we still need to report it
		requeue, updateErr := r.updateRoutingStatus(
			logger,
			ctx,
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
			ctx,
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

	if err := r.reconcile(ctx, logger, instance); err != nil {
		logger.Error(err, "Deployment reconcile failed")
		return reconcileResult, err
	}

	return reconcileResult, nil
}

func (r *ReconcileWebhookRelayForward) updateRoutingStatus(
	logger logr.Logger,
	ctx context.Context,
	status forwardv1.RoutingStatus,
	message string,
	instance *forwardv1.WebhookRelayForward) (bool, error) {
	logger.Info("Updating routing status",
		"phase", status,
		"message", message,
	)
	return r.patchStatus(ctx, instance, func(current *forwardv1.WebhookRelayForwardStatus, generation int64) {
		current.RoutingStatus = status
		current.Message = message
		conditionStatus := metav1.ConditionFalse
		reason := "RoutingFailed"
		if status == forwardv1.RoutingStatusConfigured {
			conditionStatus = metav1.ConditionTrue
			reason = "RoutingConfigured"
		}
		meta.SetStatusCondition(&current.Conditions, metav1.Condition{
			Type: forwardv1.ConditionRoutingReady, Status: conditionStatus, Reason: reason,
			Message: message, ObservedGeneration: generation,
		})
	})
}

func (r *ReconcileWebhookRelayForward) updateDeploymentStatus(
	ctx context.Context,
	logger logr.Logger,
	status forwardv1.AgentStatus,
	ready bool,
	reason string,
	message string,
	instance *forwardv1.WebhookRelayForward,
) (bool, error) {
	logger.Info("Updating deployment status",
		"status", status,
		"ready", ready,
	)
	return r.patchStatus(ctx, instance, func(current *forwardv1.WebhookRelayForwardStatus, generation int64) {
		current.AgentStatus = status
		conditionStatus := metav1.ConditionFalse
		if ready {
			conditionStatus = metav1.ConditionTrue
		}
		meta.SetStatusCondition(&current.Conditions, metav1.Condition{
			Type: forwardv1.ConditionAgentReady, Status: conditionStatus, Reason: reason,
			Message: message, ObservedGeneration: generation,
		})
	})
}

func (r *ReconcileWebhookRelayForward) patchStatus(
	ctx context.Context,
	instance *forwardv1.WebhookRelayForward,
	mutate func(*forwardv1.WebhookRelayForwardStatus, int64),
) (bool, error) {
	updated := false
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &forwardv1.WebhookRelayForward{}
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(instance), current); err != nil {
			return err
		}
		base := current.DeepCopy()
		mutate(&current.Status, current.Generation)
		current.Status.ObservedGeneration = current.Generation
		setAggregateReady(&current.Status, current.Generation)
		if reflect.DeepEqual(base.Status, current.Status) {
			instance.Status = current.Status
			return nil
		}
		if err := r.client.Status().Patch(ctx, current, client.MergeFrom(base)); err != nil {
			return err
		}
		updated = true
		instance.Status = current.Status
		return nil
	})
	return updated, err
}

func setAggregateReady(status *forwardv1.WebhookRelayForwardStatus, generation int64) {
	routing := meta.FindStatusCondition(status.Conditions, forwardv1.ConditionRoutingReady)
	agent := meta.FindStatusCondition(status.Conditions, forwardv1.ConditionAgentReady)
	ready := routing != nil && routing.ObservedGeneration == generation && routing.Status == metav1.ConditionTrue &&
		agent != nil && agent.ObservedGeneration == generation && agent.Status == metav1.ConditionTrue
	reason := "RoutingNotReady"
	message := "routing configuration is not ready"
	if routing != nil && routing.ObservedGeneration == generation && routing.Status == metav1.ConditionTrue {
		reason = "AgentNotReady"
		message = "agent Deployment is not ready"
	}
	conditionStatus := metav1.ConditionFalse
	if ready {
		conditionStatus = metav1.ConditionTrue
		reason = "Ready"
		message = "routing and agent Deployment are ready"
	}
	status.Ready = ready
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type: forwardv1.ConditionReady, Status: conditionStatus, Reason: reason,
		Message: message, ObservedGeneration: generation,
	})
}

func (r *ReconcileWebhookRelayForward) updatePublicEndpoints(ctx context.Context, logger logr.Logger, instance *forwardv1.WebhookRelayForward) (bool, error) {

	patch, update := r.shouldUpdatePublicEndpoints(instance)
	if !update {
		return false, nil
	}

	logger.Info("Updating public endpoints list",
		"endpoints", patch.Status.PublicEndpoints,
	)
	desired := append([]string(nil), patch.Status.PublicEndpoints...)
	return r.patchStatus(ctx, instance, func(current *forwardv1.WebhookRelayForwardStatus, _ int64) {
		current.PublicEndpoints = desired
	})
}

func (r *ReconcileWebhookRelayForward) reconcile(ctx context.Context, logger logr.Logger, instance *forwardv1.WebhookRelayForward) error {

	// Define a new Deployment object
	deployment := r.newDeploymentForCR(instance)

	// Set WebhookRelayForward instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, deployment, r.scheme); err != nil {
		return err
	}

	// Check if this Deployment already exists
	found := &appsv1.Deployment{}
	err := r.client.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating a new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		err = r.client.Create(ctx, deployment)
		if err != nil {
			r.recorder.Eventf(
				instance,
				nil,
				corev1.EventTypeWarning,
				"FailedCreation",
				"CreatingDeployment",
				err.Error(),
			)

			_, updateErr := r.updateDeploymentStatus(ctx, logger, forwardv1.AgentStatusCreating, false,
				"DeploymentCreateFailed", "failed to create agent Deployment", instance)
			if updateErr != nil {
				return fmt.Errorf("failed to create Deployment: %v; failed to update status: %w", err, updateErr)
			}
			return err
		}

		_, updateErr := r.updateDeploymentStatus(ctx, logger, forwardv1.AgentStatusCreating, false,
			"DeploymentCreating", "agent Deployment was created and is waiting for rollout", instance)
		if updateErr != nil {
			return updateErr
		}

		// Deployment created successfully - don't requeue
		return nil
	} else if err != nil {
		return err
	}

	// compare image, buckets
	patched, equals := r.checkDeployment(instance, found)
	if equals {
		ready, reason, message := deploymentReadiness(found)
		agentStatus := forwardv1.AgentStatusCreating
		if ready {
			agentStatus = forwardv1.AgentStatusRunning
		}
		updated, updateErr := r.updateDeploymentStatus(ctx, logger, agentStatus, ready, reason, message, instance)
		if updateErr != nil {
			return updateErr
		}
		if updated {
			return nil
		}

		_, updateErr = r.updatePublicEndpoints(ctx, logger, instance)
		if updateErr != nil {
			return updateErr
		}

		// Deployment already exists - don't requeue
		return nil
	}

	err = r.client.Update(ctx, patched)
	if err != nil {
		r.recorder.Eventf(
			instance,
			nil,
			corev1.EventTypeWarning,
			"FailedUpdate",
			"UpdatingDeployment",
			err.Error(),
		)
		return fmt.Errorf("failed to update Deployment: %s", err)
	}

	logger.Info("Deployment updated")
	_, updateErr := r.updateDeploymentStatus(ctx, logger, forwardv1.AgentStatusCreating, false,
		"DeploymentUpdating", "agent Deployment is rolling out an updated specification", instance)
	return updateErr
}

func deploymentReadiness(deployment *appsv1.Deployment) (ready bool, reason, message string) {
	for i := range deployment.Status.Conditions {
		condition := deployment.Status.Conditions[i]
		if condition.Type == appsv1.DeploymentProgressing && condition.Status == corev1.ConditionFalse &&
			condition.Reason == reasonProgressDeadlineExceeded {
			return false, reasonProgressDeadlineExceeded, "agent Deployment exceeded its progress deadline"
		}
	}
	if deployment.Status.ObservedGeneration < deployment.Generation {
		return false, "DeploymentNotObserved", "agent Deployment controller has not observed the latest generation"
	}
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	if deployment.Status.ReadyReplicas != desired || deployment.Status.UpdatedReplicas != desired ||
		deployment.Status.AvailableReplicas != desired || deployment.Status.UnavailableReplicas != 0 {
		return false, "DeploymentUnavailable", "agent Deployment rollout is not available"
	}
	return true, reasonDeploymentAvailable, "agent Deployment rollout is available"
}
