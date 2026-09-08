package webhookrelayforward

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	"github.com/webhookrelay/webhookrelay-go"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

const (
	bucketAuthTypeNone  = "none"
	bucketAuthTypeBasic = "basic"
	bucketAuthTypeToken = "token"
)

func (r *ReconcileWebhookRelayForward) ensureBucketConfiguration(logger logr.Logger, instance *forwardv1.WebhookRelayForward) error {
	var (
		err    error
		errors []string
	)

	buckets, err := r.apiClient.client.ListBuckets(&webhookrelay.BucketListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list buckets, error: %w", err)
	}
	// Updating buckets cache
	r.apiClient.bucketsCache.Set(buckets)

	for i := range instance.Spec.Buckets {
		bucketSpec := instance.Spec.Buckets[i].DeepCopy()
		if bucketSpec.Description == "" {
			bucketSpec.Description = getBucketDescription(instance)
		}
		desiredAuth, authErr := r.bucketAuthFromSpec(instance.GetNamespace(), bucketSpec.Auth)
		if authErr != nil {
			errors = append(errors, fmt.Sprintf("bucket %q: %v", bucketSpec.Name, authErr))
			continue
		}

		existingBucket, ok := getBucketByName(bucketSpec.Name, buckets)
		if !ok {
			created, err := r.apiClient.client.CreateBucket(&webhookrelay.BucketCreateOptions{
				Name:        bucketSpec.Name,
				Description: bucketSpec.Description,
			})
			if err != nil {
				logger.Error(err, "failed to create bucket",
					"bucket_ref", bucketSpec.Name,
				)
				errors = append(errors, fmt.Sprintf("create bucket %q: %v", bucketSpec.Name, err))
				continue
			} else {
				r.apiClient.bucketsCache.Add(created)
			}
			existingBucket = created
		}

		// Check if equal
		if bucketEqual(bucketSpec, existingBucket, desiredAuth) {
			// Bucket is matching the spec, nothing to do
			continue
		}
		// Bucket has changed, requires an update
		updated, err := r.apiClient.client.UpdateBucket(patchBucketFromSpec(existingBucket, bucketSpec, desiredAuth))
		if err != nil {
			logger.Error(err, "failed to update bucket",
				"bucket_ref", bucketSpec.Name,
			)
			errors = append(errors, fmt.Sprintf("update bucket %q: %v", bucketSpec.Name, err))
		} else {
			r.apiClient.bucketsCache.Add(updated)
			logger.Info("bucket updated to match the spec",
				"bucket_ref", bucketSpec.Name,
			)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to configure one or more buckets: %s", strings.Join(errors, ", "))
	}

	return nil
}

func getBucketDescription(instance *forwardv1.WebhookRelayForward) string {
	return fmt.Sprintf("Auto-created bucket by the operator for %s/%s", instance.GetNamespace(), instance.GetName())
}

func getBucketByName(name string, buckets []*webhookrelay.Bucket) (*webhookrelay.Bucket, bool) {
	for i := range buckets {
		if buckets[i].Name == name {
			return buckets[i], true
		}
	}
	return nil, false
}

func bucketEqual(spec *forwardv1.BucketSpec, bucket *webhookrelay.Bucket, desiredAuth *webhookrelay.BucketAuth) bool {
	if spec.Description != bucket.Description {
		return false
	}
	if spec.Stream != nil && *spec.Stream != bucket.Stream {
		return false
	}
	if spec.Ephemeral != nil && *spec.Ephemeral != bucket.Ephemeral {
		return false
	}
	if spec.LargeWebhooks != nil && *spec.LargeWebhooks != bucket.LargeWebhooks {
		return false
	}
	if spec.StaticIP != nil && *spec.StaticIP != bucket.StaticIP {
		return false
	}
	if desiredAuth != nil && !bucketAuthEqual(desiredAuth, &bucket.Auth) {
		return false
	}

	return true
}

func bucketAuthEqual(desired, current *webhookrelay.BucketAuth) bool {
	return desired.Type == current.Type &&
		desired.Username == current.Username &&
		desired.Password == current.Password &&
		desired.Token == current.Token
}

func patchBucketFromSpec(bucket *webhookrelay.Bucket, spec *forwardv1.BucketSpec, desiredAuth *webhookrelay.BucketAuth) *webhookrelay.Bucket {
	updated := new(webhookrelay.Bucket)
	*updated = *bucket

	updated.Description = spec.Description
	if spec.Stream != nil {
		updated.Stream = *spec.Stream
	}
	if spec.Ephemeral != nil {
		updated.Ephemeral = *spec.Ephemeral
	}
	if spec.LargeWebhooks != nil {
		updated.LargeWebhooks = *spec.LargeWebhooks
	}
	if spec.StaticIP != nil {
		updated.StaticIP = *spec.StaticIP
	}
	if desiredAuth != nil {
		// Preserve server-owned auth metadata while replacing only declarative fields.
		updated.Auth.Type = desiredAuth.Type
		updated.Auth.Username = desiredAuth.Username
		updated.Auth.Password = desiredAuth.Password
		updated.Auth.Token = desiredAuth.Token
	}

	return updated
}

func (r *ReconcileWebhookRelayForward) bucketAuthFromSpec(namespace string, spec *forwardv1.BucketAuthSpec) (*webhookrelay.BucketAuth, error) {
	if spec == nil {
		return nil, nil
	}
	auth := &webhookrelay.BucketAuth{}
	switch spec.Type {
	case bucketAuthTypeNone:
		if spec.Username != "" || spec.SecretKeyRef != nil {
			return nil, fmt.Errorf("auth type none cannot set username or secretKeyRef")
		}
		auth.Type = webhookrelay.AuthTypeNone
		return auth, nil
	case bucketAuthTypeBasic:
		if spec.Username == "" {
			return nil, fmt.Errorf("auth type basic requires username")
		}
		auth.Type = webhookrelay.AuthTypeBasic
		auth.Username = spec.Username
	case bucketAuthTypeToken:
		if spec.Username != "" {
			return nil, fmt.Errorf("auth type token cannot set username")
		}
		auth.Type = webhookrelay.AuthTypeToken
	default:
		return nil, fmt.Errorf("unsupported auth type %q", spec.Type)
	}
	if spec.SecretKeyRef == nil || spec.SecretKeyRef.Name == "" || spec.SecretKeyRef.Key == "" {
		return nil, fmt.Errorf("auth type %s requires secretKeyRef name and key", spec.Type)
	}
	secret := &corev1.Secret{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: spec.SecretKeyRef.Name}, secret); err != nil {
		return nil, fmt.Errorf("read authentication Secret %q: %w", spec.SecretKeyRef.Name, err)
	}
	value, ok := secret.Data[spec.SecretKeyRef.Key]
	if !ok || len(value) == 0 {
		return nil, fmt.Errorf("authentication Secret %q has no non-empty %q key", spec.SecretKeyRef.Name, spec.SecretKeyRef.Key)
	}
	if spec.Type == bucketAuthTypeBasic {
		auth.Password = string(value)
	} else {
		auth.Token = string(value)
	}
	return auth, nil
}
