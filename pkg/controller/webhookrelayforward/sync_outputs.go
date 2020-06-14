package webhookrelayforward

import (
	"github.com/go-logr/logr"

	"github.com/webhookrelay/webhookrelay-go"
	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

func (r *ReconcileWebhookRelayForward) ensureBucketOutputs(logger logr.Logger, instance *forwardv1.WebhookRelayForward, bucket *forwardv1.BucketSpec) error {

	return nil
}

type outputsDiff struct {
	create []*webhookrelay.Output
	update []*webhookrelay.Output
	delete []*webhookrelay.Output
}

func getOutputsDiff(current, desired []*webhookrelay.Output) *outputsDiff {

	diff := &outputsDiff{}

	currentMap := make(map[string]*webhookrelay.Output)

	for i := range current {
		currentMap[current[i].Name] = current[i]
	}

	for i := range desired {
		currentOutput, ok := currentMap[desired[i].Name]
		if !ok {
			diff.create = append(diff.create, desired[i])
			continue
		}
		if outputsEqual(currentOutput, desired[i]) {
			// Nothing to do
			continue
		}
		// Setting ID and adding to the update list
		desired[i].ID = currentOutput.ID
		diff.update = append(diff.update, desired[i])
	}

	// TODO: check for outputs to delete,

	return diff
}

func desiredOutputs(bucketSpec *forwardv1.BucketSpec, bucket *webhookrelay.Bucket) []*webhookrelay.Output {

	var desired []*webhookrelay.Output

	for i := range bucketSpec.Outputs {
		desired = append(desired, inputSpecToOutput(&bucketSpec.Outputs[i], bucket))
	}

	return desired
}

func inputSpecToOutput(spec *forwardv1.OutputSpec, bucket *webhookrelay.Bucket) *webhookrelay.Output {
	header := make(map[string][]string)

	internal := true

	if spec.Internal != nil {
		// set, checking val
		internal = *spec.Internal
	}

	if spec.OverrideHeaders != nil {
		for k, v := range spec.OverrideHeaders {
			header[k] = []string{v}
		}
	}

	return &webhookrelay.Output{
		Name:        spec.Name,
		BucketID:    bucket.ID,
		FunctionID:  spec.FunctionID,
		Headers:     header,
		Destination: spec.Destination,
		Timeout:     spec.Timeout,
		Internal:    internal,
		Description: spec.Description,
	}
}

func outputsEqual(current, desired *webhookrelay.Output) bool {

	if current.Name != desired.Name {
		return false
	}
	if current.FunctionID != desired.FunctionID {
		return false
	}
	if current.Destination != desired.Destination {
		return false
	}

	if len(current.Headers) != len(desired.Headers) {
		for k := range current.Headers {
			if !sliceEqual(current.Headers[k], desired.Headers[k]) {
				return false
			}
		}
	}

	if current.Internal != desired.Internal {
		return false
	}

	if current.Timeout != desired.Timeout {
		return false
	}

	if current.Description != desired.Description {
		return false
	}

	return true
}
