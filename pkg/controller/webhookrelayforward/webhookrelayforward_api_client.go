package webhookrelayforward

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/webhookrelay/webhookrelay-go"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

// Errors
var (
	ErrCredentialsNotProvided        = errors.New("access token key and secret not provided")
	ErrCrossNamespaceSecretReference = errors.New("cross-namespace credential secret references are not allowed")
)

// WebhookRelayClient is a wrapper for the Webhook Relay API client
type WebhookRelayClient struct {
	// client is Webhook Relay API client.
	client             *webhookrelay.API
	instanceName       string
	instanceGeneration int64
	// instanceUID is the UID of the NodeForwarder the client belongs to.
	instanceUID types.UID

	// Preserving access token as we will need them for the
	// webhookrelayd deployments.
	// TODO: provision new access token key & secret pair with limited
	// API access
	accessTokenKey    string
	accessTokenSecret string

	bucketsCache *bucketsCache
}

func (r *ReconcileWebhookRelayForward) setClientForCluster(ctx context.Context, instance *forwardv1.WebhookRelayForward) error {
	// credentials to use
	var (
		relayKey    string
		relaySecret string
	)

	if instance.Spec.SecretRefName != "" {
		namespace, err := credentialsSecretNamespace(instance)
		if err != nil {
			return err
		}

		// Obtain the Webhook Relay API access token key and secret to be used in the client.
		secretNamespacedName := types.NamespacedName{
			Namespace: namespace,
			Name:      instance.Spec.SecretRefName,
		}
		secretInstance := &corev1.Secret{}
		err = r.client.Get(ctx, secretNamespacedName, secretInstance)
		if err != nil {
			return err
		}

		relayKey = string(secretInstance.Data[forwardv1.AccessTokenKeyName])
		relaySecret = string(secretInstance.Data[forwardv1.AccessTokenSecretName])
	} else if r.config.Relay.Key != "" && r.config.Relay.Secret != "" {
		// using operator config
		relayKey = r.config.Relay.Key
		relaySecret = r.config.Relay.Secret
	} else {
		return ErrCredentialsNotProvided
	}

	options := []webhookrelay.Option{webhookrelay.WithAPIEndpointURL(r.config.APIEndpointURL)}

	if r.config.HTTPSPRoxy != "" {
		proxyURL, err := url.Parse(r.config.HTTPSPRoxy)
		if err != nil {
			return fmt.Errorf("failed to configure API client, proxy URL '%s' could not be parsed: %w", r.config.HTTPSPRoxy, err)
		}
		// Creating a new HTTP client
		transport := &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		}

		httpClient := &http.Client{
			Transport: transport,
		}

		options = append(options, webhookrelay.WithHTTPClient(httpClient))
	}

	apiClient, err := webhookrelay.New(relayKey, relaySecret, options...)
	if err != nil {
		return err
	}

	r.apiClient = &WebhookRelayClient{
		client:             apiClient,
		instanceName:       instance.GetName(),
		instanceGeneration: instance.GetGeneration(),
		instanceUID:        instance.GetUID(),
		// setting credentials that can be reused for deployments
		accessTokenKey:    relayKey,
		accessTokenSecret: relaySecret,
		bucketsCache:      newBucketsCache(),
	}

	return nil
}

func credentialsSecretNamespace(instance *forwardv1.WebhookRelayForward) (string, error) {
	namespace := instance.GetNamespace()
	if instance.Spec.SecretRefNamespace != "" && instance.Spec.SecretRefNamespace != namespace {
		return "", fmt.Errorf("%w: WebhookRelayForward %s/%s cannot read Secret %s/%s",
			ErrCrossNamespaceSecretReference,
			instance.GetNamespace(), instance.GetName(),
			instance.Spec.SecretRefNamespace, instance.Spec.SecretRefName,
		)
	}
	return namespace, nil
}
