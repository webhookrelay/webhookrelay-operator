package webhookrelayforward

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	webhookrelay "github.com/webhookrelay/webhookrelay-go"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

const (
	statusTestName      = "example"
	statusTestNamespace = "default"
)

func TestDeploymentReadiness(t *testing.T) {
	replicas := int32(1)
	tests := []struct {
		name       string
		deployment *appsv1.Deployment
		ready      bool
		reason     string
		desired    int32
	}{
		{
			name: "generation not observed",
			deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.DeploymentSpec{Replicas: &replicas}},
			reason:  reasonDeploymentNotObserved,
			desired: 1,
		},
		{
			name: "partial rollout",
			deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec:   appsv1.DeploymentSpec{Replicas: &replicas},
				Status: appsv1.DeploymentStatus{ObservedGeneration: 2, UpdatedReplicas: 1}},
			reason:  reasonDeploymentUnavailable,
			desired: 1,
		},
		{
			name: "progress deadline exceeded",
			deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.DeploymentSpec{Replicas: &replicas},
				Status: appsv1.DeploymentStatus{ObservedGeneration: 2, Conditions: []appsv1.DeploymentCondition{{
					Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: reasonProgressDeadlineExceeded,
				}}}},
			reason:  reasonProgressDeadlineExceeded,
			desired: 1,
		},
		{
			name: "available",
			deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.DeploymentSpec{Replicas: &replicas},
				Status: appsv1.DeploymentStatus{ObservedGeneration: 2, ReadyReplicas: 1,
					UpdatedReplicas: 1, AvailableReplicas: 1}},
			ready: true, reason: reasonDeploymentAvailable, desired: 1,
		},
		{
			name: "zero observed replicas cannot satisfy desired state",
			deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Status: appsv1.DeploymentStatus{ObservedGeneration: 2}},
			reason: reasonDeploymentUnavailable, desired: 1,
		},
		{
			name: "stale progress deadline condition",
			deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 3},
				Status: appsv1.DeploymentStatus{ObservedGeneration: 2, Conditions: []appsv1.DeploymentCondition{{
					Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: reasonProgressDeadlineExceeded,
				}}}},
			reason: reasonDeploymentNotObserved, desired: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ready, reason, _ := deploymentReadiness(test.deployment, test.desired)
			assert.Equal(t, test.ready, ready)
			assert.Equal(t, test.reason, reason)
		})
	}
}

func TestPatchStatusRejectsNewerStoredGeneration(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, forwardv1.AddToScheme(scheme))
	stored := &forwardv1.WebhookRelayForward{ObjectMeta: metav1.ObjectMeta{
		Name: statusTestName, Namespace: statusTestNamespace, Generation: 4,
	}}
	stale := stored.DeepCopy()
	stale.Generation = 3
	baseClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&forwardv1.WebhookRelayForward{}).WithObjects(stored).Build()
	reconciler := &ReconcileWebhookRelayForward{client: baseClient}

	updated, err := reconciler.updateRoutingStatus(log, context.Background(),
		forwardv1.RoutingStatusConfigured, "", stale)

	assert.ErrorIs(t, err, errStatusGenerationChanged)
	assert.False(t, updated)
	require.NoError(t, baseClient.Get(context.Background(), client.ObjectKeyFromObject(stored), stored))
	assert.Zero(t, stored.Status.ObservedGeneration)
	assert.Empty(t, stored.Status.Conditions)
}

func TestReconcileReturnsRoutingStatusWriteFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	t.Cleanup(server.Close)
	apiClient, err := webhookrelay.New("test-key", "test-secret", webhookrelay.WithAPIEndpointURL(server.URL))
	require.NoError(t, err)
	scheme := runtime.NewScheme()
	require.NoError(t, forwardv1.AddToScheme(scheme))
	instance := &forwardv1.WebhookRelayForward{ObjectMeta: metav1.ObjectMeta{
		Name: statusTestName, Namespace: statusTestNamespace, Generation: 1, UID: types.UID("test-uid"),
	}}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&forwardv1.WebhookRelayForward{}).WithObjects(instance).Build()
	reconciler := &ReconcileWebhookRelayForward{
		client: &failingStatusClient{Client: baseClient},
		apiClient: &WebhookRelayClient{client: apiClient, instanceName: instance.Name,
			instanceGeneration: instance.Generation, instanceUID: instance.UID, bucketsCache: newBucketsCache()},
	}

	_, err = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(instance)})

	assert.ErrorIs(t, err, assert.AnError)
}

func TestSetAggregateReadyRequiresCurrentRoutingAndAgentConditions(t *testing.T) {
	status := forwardv1.WebhookRelayForwardStatus{Conditions: []metav1.Condition{
		{Type: forwardv1.ConditionRoutingReady, Status: metav1.ConditionTrue, Reason: "RoutingConfigured", ObservedGeneration: 3},
		{Type: forwardv1.ConditionAgentReady, Status: metav1.ConditionTrue, Reason: "DeploymentAvailable", ObservedGeneration: 2},
	}}

	setAggregateReady(&status, 3)
	ready := meta.FindStatusCondition(status.Conditions, forwardv1.ConditionReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Equal(t, "AgentNotReady", ready.Reason)
	assert.False(t, status.Ready)

	status.Conditions[1].ObservedGeneration = 3
	setAggregateReady(&status, 3)
	ready = meta.FindStatusCondition(status.Conditions, forwardv1.ConditionReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionTrue, ready.Status)
	assert.True(t, status.Ready)
	transition := ready.LastTransitionTime
	setAggregateReady(&status, 3)
	assert.Equal(t, transition, meta.FindStatusCondition(status.Conditions, forwardv1.ConditionReady).LastTransitionTime)
}

func TestPatchStatusRetriesConflictAndPreservesLegacyMirrors(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, forwardv1.AddToScheme(scheme))
	instance := &forwardv1.WebhookRelayForward{ObjectMeta: metav1.ObjectMeta{
		Name: statusTestName, Namespace: statusTestNamespace, Generation: 4,
	}}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&forwardv1.WebhookRelayForward{}).WithObjects(instance).Build()
	conflicts := &conflictStatusClient{Client: baseClient}
	reconciler := &ReconcileWebhookRelayForward{client: conflicts}

	updated, err := reconciler.updateRoutingStatus(log, context.Background(),
		forwardv1.RoutingStatusConfigured, "", instance)

	require.NoError(t, err)
	assert.True(t, updated)
	assert.Equal(t, 2, conflicts.attempts)
	stored := &forwardv1.WebhookRelayForward{}
	require.NoError(t, baseClient.Get(context.Background(), client.ObjectKeyFromObject(instance), stored))
	assert.Equal(t, int64(4), stored.Status.ObservedGeneration)
	assert.Equal(t, forwardv1.RoutingStatusConfigured, stored.Status.RoutingStatus)
	assert.False(t, stored.Status.Ready)
	condition := meta.FindStatusCondition(stored.Status.Conditions, forwardv1.ConditionRoutingReady)
	require.NotNil(t, condition)
	assert.Equal(t, metav1.ConditionTrue, condition.Status)
	assert.Equal(t, int64(4), condition.ObservedGeneration)
	transition := condition.LastTransitionTime

	updated, err = reconciler.updateRoutingStatus(log, context.Background(),
		forwardv1.RoutingStatusConfigured, "", instance)
	require.NoError(t, err)
	assert.False(t, updated)
	assert.Equal(t, 2, conflicts.attempts)
	require.NoError(t, baseClient.Get(context.Background(), client.ObjectKeyFromObject(instance), stored))
	assert.Equal(t, transition,
		meta.FindStatusCondition(stored.Status.Conditions, forwardv1.ConditionRoutingReady).LastTransitionTime)
}

type conflictStatusClient struct {
	client.Client
	attempts int
}

func (c *conflictStatusClient) Status() client.SubResourceWriter {
	return &conflictStatusWriter{SubResourceWriter: c.Client.Status(), client: c}
}

type conflictStatusWriter struct {
	client.SubResourceWriter
	client *conflictStatusClient
}

type failingStatusClient struct {
	client.Client
}

func (c *failingStatusClient) Status() client.SubResourceWriter {
	return &failingStatusWriter{SubResourceWriter: c.Client.Status()}
}

type failingStatusWriter struct {
	client.SubResourceWriter
}

func (w *failingStatusWriter) Patch(
	_ context.Context,
	_ client.Object,
	_ client.Patch,
	_ ...client.SubResourcePatchOption,
) error {
	return assert.AnError
}

func (w *conflictStatusWriter) Patch(
	ctx context.Context,
	obj client.Object,
	patch client.Patch,
	opts ...client.SubResourcePatchOption,
) error {
	w.client.attempts++
	if w.client.attempts == 1 {
		return apierrors.NewConflict(schema.GroupResource{Group: forwardv1.SchemeGroupVersion.Group,
			Resource: "webhookrelayforwards"}, obj.GetName(), assert.AnError)
	}
	return w.SubResourceWriter.Patch(ctx, obj, patch, opts...)
}
