package webhookrelayforward

import (
	"fmt"
	"strings"

	"github.com/webhookrelay/webhookrelay-go"

	"github.com/go-logr/logr"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

// ensureRoutingConfiguration check buckets, inputs and outputs on the Webhook Relay server side. If something needs to be
// changed - it performs necessary configuration changes
func (r *ReconcileWebhookRelayForward) ensureRoutingConfiguration(logger logr.Logger, instance *forwardv1.WebhookRelayForward) error {

	var err error

	err = r.ensureBucketConfiguration(logger, instance)
	if err != nil {
		return err
	}

	// Configuring bucket inputs and outputs. Here, errors can happen mostly due to user error when
	// invalid values are set, however we can still continue as most of the input/output updates should succeed
	for idx := range instance.Spec.Buckets {
		err = r.ensureBucketInputs(logger, instance, &instance.Spec.Buckets[idx])
		if err != nil {
			logger.Error(err, "failed to configure bucket '%s' inputs", instance.Spec.Buckets[idx].Ref)
		}

		err = r.ensureBucketOutputs(logger, instance, &instance.Spec.Buckets[idx])
		if err != nil {
			logger.Error(err, "failed to configure bucket '%s' outputs", instance.Spec.Buckets[idx].Ref)
		}
	}

	return nil
}

func (r *ReconcileWebhookRelayForward) ensureBucketConfiguration(logger logr.Logger, instance *forwardv1.WebhookRelayForward) error {
	var (
		err    error
		errors []string
	)

	buckets, err := r.apiClient.client.ListBuckets(&webhookrelay.BucketListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list buckets, error: %w", err)
	}

	for i := range instance.Spec.Buckets {

		if instance.Spec.Buckets[i].Description == "" {
			instance.Spec.Buckets[i].Description = getBucketDescription(instance)
		}

		existingBucket, ok := getBucketByRef(instance.Spec.Buckets[i].Ref, buckets)
		if !ok {
			// Create a new bucket based on the provided BucketSpec
			// TODO: add authentication settings to CRD (https://github.com/webhookrelay/webhookrelay-operator/issues/2)
			_, err = r.apiClient.client.CreateBucket(&webhookrelay.BucketCreateOptions{
				Name:        instance.Spec.Buckets[i].Ref,
				Description: instance.Spec.Buckets[i].Description,
			})
			if err != nil {
				logger.Error(err, "failed to create bucket",
					"bucket_ref", instance.Spec.Buckets[i].Ref,
				)
			}
			continue
		}

		// Check if equal
		if bucketEqual(&instance.Spec.Buckets[i], existingBucket) {
			// Bucket is matching the spec, nothing to do
			continue
		}

		// Bucket has changed, requires an update
		_, err = r.apiClient.client.UpdateBucket(patchBucketFromSpec(existingBucket, &instance.Spec.Buckets[i]))
		if err != nil {
			logger.Error(err, "failed to update bucket",
				"bucket_ref", instance.Spec.Buckets[i].Ref,
			)
		} else {
			logger.Info("bucket updated to match the spec",
				"bucket_ref", instance.Spec.Buckets[i].Ref,
			)
		}

	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to configure one or more buckets: %s", strings.Join(errors, ", "))
	}

	return nil
}

// ensureBucketInputs checks and configures input specific information
func (r *ReconcileWebhookRelayForward) ensureBucketInputs(logger logr.Logger, instance *forwardv1.WebhookRelayForward, bucket *forwardv1.BucketSpec) error {

	return nil
}

func (r *ReconcileWebhookRelayForward) ensureBucketOutputs(logger logr.Logger, instance *forwardv1.WebhookRelayForward, bucket *forwardv1.BucketSpec) error {

	return nil
}

func getBucketDescription(instance *forwardv1.WebhookRelayForward) string {
	return fmt.Sprintf("Auto-created bucket by the operator for %s/%s", instance.GetNamespace(), instance.GetName())
}

func getBucketByRef(ref string, buckets []*webhookrelay.Bucket) (*webhookrelay.Bucket, bool) {
	for i := range buckets {
		if buckets[i].Name == ref || buckets[i].ID == ref {
			return buckets[i], true
		}
	}
	return nil, false
}

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
