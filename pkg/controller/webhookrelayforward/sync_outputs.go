package webhookrelayforward

import (
	"fmt"

	"github.com/go-logr/logr"

	"github.com/webhookrelay/webhookrelay-go"
	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

func (r *ReconcileWebhookRelayForward) ensureBucketOutputs(logger logr.Logger, instance *forwardv1.WebhookRelayForward, bucketSpec *forwardv1.BucketSpec) error {

	// If no outputs are defined, nothing to do
	if len(bucketSpec.Outputs) == 0 {
		return nil
	}

	bucket, ok := r.apiClient.bucketsCache.Get(bucketSpec.Name)
	if !ok {
		return fmt.Errorf("bucket '%s' not found in the cache, will wait for the next reconcile loop", bucketSpec.Name)
	}

	logger = logger.WithValues(
		"bucket_name", bucket.Name,
		"bucket_id", bucket.ID,
	)

	// Create a list of desired outputs and then diff existing
	// ones against them to build a list of what outputs
	// we should create, update and which ones to delete
	desired := desiredOutputs(bucketSpec, bucket)
	diff := getOutputsDiff(bucket.Outputs, desired)

	var err error

	// Create inputs that need to be created
	for idx := range diff.create {
		logger.Info("creating output",
			"output_id", diff.create[idx].ID,
			"output_name", diff.create[idx].Name,
		)
		_, err = r.apiClient.client.CreateOutput(diff.create[idx])
		if err != nil {
			logger.Error(err, "failed to create output")
		}
	}

	for idx := range diff.update {
		logger.Info("updating output",
			"output_id", diff.update[idx].ID,
			"output_name", diff.update[idx].Name,
		)
		_, err = r.apiClient.client.UpdateOutput(diff.update[idx])
		if err != nil {
			logger.Error(err, "failed to update input",
				"input_id", diff.update[idx].ID,
			)
		}
	}

	for idx := range diff.delete {
		logger.Info("deleting output",
			"output_id", diff.delete[idx].ID,
			"output_name", diff.delete[idx].Name,
		)
		err = r.apiClient.client.DeleteOutput(&webhookrelay.OutputDeleteOptions{
			Bucket: diff.delete[idx].BucketID,
			Output: diff.delete[idx].ID,
		})
		if err != nil {
			logger.Error(err, "failed to delete output",
				"output_id", diff.update[idx].ID,
			)
		}
	}

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

			// Deleting entry from the map, what's left in the map
			// will only be the outputs that shouldn't be there
			// anymore
			delete(currentMap, currentOutput.Name)
			continue
		}
		// Setting ID and adding to the update list
		desired[i].ID = currentOutput.ID
		diff.update = append(diff.update, desired[i])

		// Deleting entry from the map, what's left in the map
		// will only be the outputs that shouldn't be there
		// anymore
		delete(currentMap, currentOutput.Name)
	}
	// Collecting leftovers for deletion
	for _, v := range currentMap {
		diff.delete = append(diff.delete, v)
	}
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
