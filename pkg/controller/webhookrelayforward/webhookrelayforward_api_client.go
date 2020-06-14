package webhookrelayforward

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/webhookrelay/webhookrelay-go"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

// Errors
var (
	ErrCredentialsNotProvided = errors.New("Webhook Relay access token key and secret not provided")
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

func (r *ReconcileWebhookRelayForward) setClientForCluster(instance *forwardv1.WebhookRelayForward) error {

	// credentials to use
	var (
		relayKey    string
		relaySecret string
	)

	if instance.Spec.SecretRefName != "" {

		namespace := instance.Spec.SecretRefNamespace
		if namespace == "" {
			// defaulting to CR namespace
			namespace = instance.GetNamespace()
		}

		// Obtain the Webhook Relay API access token key and secret to be used in the client.
		secretNamespacedName := types.NamespacedName{
			Namespace: namespace,
			Name:      instance.Spec.SecretRefName,
		}
		secretInstance := &corev1.Secret{}
		err := r.client.Get(context.TODO(), secretNamespacedName, secretInstance)
		if err != nil {
			return err
		}

		relayKey = string(secretInstance.Data[forwardv1.AccessTokenKeyName])
		relaySecret = string(secretInstance.Data[forwardv1.AccessTokenSecretName])
	} else if r.config.Token.Key != "" && r.config.Token.Secret != "" {
		// using operator config
		relayKey = r.config.Token.Key
		relaySecret = r.config.Token.Secret
	} else {
		return ErrCredentialsNotProvided
	}

	apiClient, err := webhookrelay.New(relayKey, relaySecret)
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
