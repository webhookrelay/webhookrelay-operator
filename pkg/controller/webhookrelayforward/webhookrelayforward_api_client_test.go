package webhookrelayforward

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

const (
	testForwardName      = "forward"
	testForwardNamespace = "tenant-a"
	testSecretObjectName = "relay-auth"
)

func TestCredentialsSecretNamespaceDefaultsToCRNamespace(t *testing.T) {
	instance := &forwardv1.WebhookRelayForward{
		ObjectMeta: metav1.ObjectMeta{Name: testForwardName, Namespace: testForwardNamespace},
		Spec:       forwardv1.WebhookRelayForwardSpec{SecretRefName: testSecretObjectName},
	}

	namespace, err := credentialsSecretNamespace(instance)
	require.NoError(t, err)
	assert.Equal(t, testForwardNamespace, namespace)
}

func TestSetClientRejectsCrossNamespaceSecretBeforeKubernetesRead(t *testing.T) {
	instance := &forwardv1.WebhookRelayForward{
		ObjectMeta: metav1.ObjectMeta{Name: testForwardName, Namespace: testForwardNamespace},
		Spec: forwardv1.WebhookRelayForwardSpec{ // #nosec G101 -- these are Kubernetes object names, not credentials.
			SecretRefName:      testSecretObjectName,
			SecretRefNamespace: "tenant-b",
		},
	}

	// A nil Kubernetes client makes this test fail with a panic if validation
	// ever moves after the Secret read.
	reconciler := &ReconcileWebhookRelayForward{}
	err := reconciler.setClientForCluster(context.Background(), instance)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCrossNamespaceSecretReference))
}
