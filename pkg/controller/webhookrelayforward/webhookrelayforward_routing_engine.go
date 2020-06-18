package webhookrelayforward

import (
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
		// first ensuring outputs, because we might need to specify output
		// ID on the input if it has "ResponseFromOutput"
		err = r.ensureBucketOutputs(logger, instance, &instance.Spec.Buckets[idx])
		if err != nil {
			logger.Error(err, "failed to configure bucket '%s' outputs", instance.Spec.Buckets[idx].Name)
		}

		err = r.ensureBucketInputs(logger, instance, &instance.Spec.Buckets[idx])
		if err != nil {
			logger.Error(err, "failed to configure bucket '%s' inputs", instance.Spec.Buckets[idx].Name)
		}

	}

	return nil
}
