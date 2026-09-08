package webhookrelayforward

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

// ensureRoutingConfiguration check buckets, inputs and outputs on the Webhook Relay server side. If something needs to be
// changed - it performs necessary configuration changes
func (r *ReconcileWebhookRelayForward) ensureRoutingConfiguration(logger logr.Logger, instance *forwardv1.WebhookRelayForward) error {
	err := r.ensureBucketConfiguration(logger, instance)
	if err != nil {
		return err
	}

	var syncErrors []string
	for idx := range instance.Spec.Buckets {
		// first ensuring outputs, because we might need to specify output
		// ID on the input if it has "ResponseFromOutput"
		err = r.ensureBucketOutputs(logger, &instance.Spec.Buckets[idx])
		if err != nil {
			logger.Error(err, "failed to configure bucket outputs", "bucket_ref", instance.Spec.Buckets[idx].Name)
			syncErrors = append(syncErrors, fmt.Sprintf("bucket %q outputs: %v", instance.Spec.Buckets[idx].Name, err))
		}

		err = r.ensureBucketInputs(logger, &instance.Spec.Buckets[idx])
		if err != nil {
			logger.Error(err, "failed to configure bucket inputs", "bucket_ref", instance.Spec.Buckets[idx].Name)
			syncErrors = append(syncErrors, fmt.Sprintf("bucket %q inputs: %v", instance.Spec.Buckets[idx].Name, err))
		}

	}

	if len(syncErrors) > 0 {
		return fmt.Errorf("failed to configure routing: %s", strings.Join(syncErrors, "; "))
	}
	return nil
}
