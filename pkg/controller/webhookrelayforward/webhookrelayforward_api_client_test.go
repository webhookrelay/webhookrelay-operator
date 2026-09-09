package webhookrelayforward

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
	"github.com/webhookrelay/webhookrelay-operator/pkg/config"
)

const (
	testForwardName      = "forward"
	testForwardNamespace = "tenant-a"
	testSecretObjectName = "relay-auth"
	testRelayKey         = "test-key"
	testRelaySecret      = "test-secret"
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

func TestSetClientUsesConfiguredAPIEndpoint(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	t.Cleanup(server.Close)

	scheme := runtime.NewScheme()
	require.NoError(t, forwardv1.AddToScheme(scheme))
	instance := &forwardv1.WebhookRelayForward{
		ObjectMeta: metav1.ObjectMeta{Name: testForwardName, Namespace: testForwardNamespace},
	}
	reconciler := &ReconcileWebhookRelayForward{
		client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		config: &config.Config{
			APIEndpointURL: server.URL,
		},
	}
	reconciler.config.Relay.Key = testRelayKey
	reconciler.config.Relay.Secret = testRelaySecret

	require.NoError(t, reconciler.setClientForCluster(context.Background(), instance))
	assert.Equal(t, server.URL, reconciler.apiClient.client.Endpoint())
	_, err := reconciler.apiClient.client.ListBuckets(nil)
	require.NoError(t, err)
	assert.NotEmpty(t, authorization)
}

func TestSetClientComposesProxyAndAPIEndpoint(t *testing.T) {
	var requestedURL string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestedURL = request.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	t.Cleanup(proxy.Close)

	scheme := runtime.NewScheme()
	require.NoError(t, forwardv1.AddToScheme(scheme))
	instance := &forwardv1.WebhookRelayForward{
		ObjectMeta: metav1.ObjectMeta{Name: testForwardName, Namespace: testForwardNamespace},
	}
	reconciler := &ReconcileWebhookRelayForward{
		client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		config: &config.Config{
			APIEndpointURL: "http://relay.invalid/v1",
			HTTPSPRoxy:     proxy.URL,
		},
	}
	reconciler.config.Relay.Key = testRelayKey
	reconciler.config.Relay.Secret = testRelaySecret

	require.NoError(t, reconciler.setClientForCluster(context.Background(), instance))
	_, err := reconciler.apiClient.client.ListBuckets(nil)
	require.NoError(t, err)
	assert.Equal(t, "http://relay.invalid/v1/buckets", requestedURL)
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
