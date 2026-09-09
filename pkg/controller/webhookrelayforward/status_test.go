package webhookrelayforward

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

func TestDeploymentReadiness(t *testing.T) {
	replicas := int32(1)
	tests := []struct {
		name       string
		deployment *appsv1.Deployment
		ready      bool
		reason     string
	}{
		{
			name: "generation not observed",
			deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.DeploymentSpec{Replicas: &replicas}},
			reason: "DeploymentNotObserved",
		},
		{
			name: "partial rollout",
			deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec:   appsv1.DeploymentSpec{Replicas: &replicas},
				Status: appsv1.DeploymentStatus{ObservedGeneration: 2, UpdatedReplicas: 1}},
			reason: "DeploymentUnavailable",
		},
		{
			name: "progress deadline exceeded",
			deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.DeploymentSpec{Replicas: &replicas},
				Status: appsv1.DeploymentStatus{Conditions: []appsv1.DeploymentCondition{{
					Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: reasonProgressDeadlineExceeded,
				}}}},
			reason: reasonProgressDeadlineExceeded,
		},
		{
			name: "available",
			deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec: appsv1.DeploymentSpec{Replicas: &replicas},
				Status: appsv1.DeploymentStatus{ObservedGeneration: 2, ReadyReplicas: 1,
					UpdatedReplicas: 1, AvailableReplicas: 1}},
			ready: true, reason: reasonDeploymentAvailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ready, reason, _ := deploymentReadiness(test.deployment)
			assert.Equal(t, test.ready, ready)
			assert.Equal(t, test.reason, reason)
		})
	}
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
		Name: "example", Namespace: "default", Generation: 4,
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
