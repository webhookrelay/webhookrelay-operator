package webhookrelayforward

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	"github.com/webhookrelay/webhookrelay-go"
	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
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
		if instance.Spec.Buckets[i].Description == "" {
			instance.Spec.Buckets[i].Description = getBucketDescription(instance)
		}

		existingBucket, ok := getBucketByName(instance.Spec.Buckets[i].Name, buckets)
		if !ok {
			// Create a new bucket based on the provided BucketSpec
			// TODO: add authentication settings to CRD (https://github.com/webhookrelay/webhookrelay-operator/issues/2)
			created, err := r.apiClient.client.CreateBucket(&webhookrelay.BucketCreateOptions{
				Name:        instance.Spec.Buckets[i].Name,
				Description: instance.Spec.Buckets[i].Description,
			})
			if err != nil {
				logger.Error(err, "failed to create bucket",
					"bucket_ref", instance.Spec.Buckets[i].Name,
				)
			} else {
				r.apiClient.bucketsCache.Add(created)
			}
			continue
		}

		// Check if equal
		if bucketEqual(&instance.Spec.Buckets[i], existingBucket) {
			// Bucket is matching the spec, nothing to do
			continue
		}
		// Bucket has changed, requires an update
		updated, err := r.apiClient.client.UpdateBucket(patchBucketFromSpec(existingBucket, &instance.Spec.Buckets[i]))
		if err != nil {
			logger.Error(err, "failed to update bucket",
				"bucket_ref", instance.Spec.Buckets[i].Name,
			)
		} else {
			r.apiClient.bucketsCache.Add(updated)
			logger.Info("bucket updated to match the spec",
				"bucket_ref", instance.Spec.Buckets[i].Name,
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

//nolint
func bucketEqual(spec *forwardv1.BucketSpec, bucket *webhookrelay.Bucket) bool {

	if spec.Description != bucket.Description {
		return false
	}

	// TODO: check auth

	return true
}

func patchBucketFromSpec(bucket *webhookrelay.Bucket, spec *forwardv1.BucketSpec) *webhookrelay.Bucket {
	updated := new(webhookrelay.Bucket)
	*updated = *bucket

	updated.Description = spec.Description
	// TODO: update name?
	// TODO: update auth?

	return updated
}
