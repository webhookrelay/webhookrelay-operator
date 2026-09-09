package webhookrelayforward

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
	"github.com/webhookrelay/webhookrelay-operator/pkg/config"
)

func TestNewDeploymentUsesAgentImageResourcesAndWebsocketTransport(t *testing.T) {
	websocket := true
	instance := &forwardv1.WebhookRelayForward{
		ObjectMeta: metav1.ObjectMeta{Name: testForwardName, Namespace: testForwardNamespace},
		Spec: forwardv1.WebhookRelayForwardSpec{ // #nosec G101 -- SecretRefName is an object name, not credential material.
			Image:              "registry.example/relay-agent:arm64",
			SecretRefName:      testSecretObjectName,
			WebsocketTransport: &websocket,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
				Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("128Mi")},
			},
			ExtraEnvVars: []corev1.EnvVar{
				{Name: containerWebsocketEnvName, Value: "false"},
				{Name: "LOG_LEVEL", Value: "debug"},
			},
		},
	}
	reconciler := &ReconcileWebhookRelayForward{config: &config.Config{}}

	deployment := reconciler.newDeploymentForCR(instance)
	require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
	container := deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "registry.example/relay-agent:arm64", container.Image)
	assert.Equal(t, resource.MustParse("100m"), container.Resources.Requests[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("128Mi"), container.Resources.Limits[corev1.ResourceMemory])
	assert.Equal(t, "true", envValue(container.Env, containerWebsocketEnvName))
	assert.Equal(t, "debug", envValue(container.Env, "LOG_LEVEL"))
	assert.Equal(t, 1, countEnv(container.Env, containerWebsocketEnvName))
}

func TestContainersEqualDetectsResourceChanges(t *testing.T) {
	current := &corev1.Container{Image: "agent", Resources: corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
	}}
	desired := current.DeepCopy()
	desired.Resources.Requests[corev1.ResourceCPU] = resource.MustParse("200m")

	assert.False(t, containersEqual(current, desired))
}

func TestCheckDeploymentRestoresScaledDownReplica(t *testing.T) {
	instance := &forwardv1.WebhookRelayForward{
		ObjectMeta: metav1.ObjectMeta{Name: testForwardName, Namespace: testForwardNamespace},
		Spec: forwardv1.WebhookRelayForwardSpec{ // #nosec G101 -- SecretRefName is an object name, not credential material.
			SecretRefName: testSecretObjectName,
		},
	}
	reconciler := &ReconcileWebhookRelayForward{config: &config.Config{}}
	current := reconciler.newDeploymentForCR(instance)
	current.Spec.Replicas = toInt32(0)

	patched, equal := reconciler.checkDeployment(instance, current)

	assert.False(t, equal)
	require.NotNil(t, patched.Spec.Replicas)
	assert.Equal(t, int32(1), *patched.Spec.Replicas)
}

func envValue(env []corev1.EnvVar, name string) string {
	for i := range env {
		if env[i].Name == name {
			return env[i].Value
		}
	}
	return ""
}

func countEnv(env []corev1.EnvVar, name string) int {
	count := 0
	for i := range env {
		if env[i].Name == name {
			count++
		}
	}
	return count
}
