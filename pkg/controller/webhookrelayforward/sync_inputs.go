package webhookrelayforward

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/webhookrelay/webhookrelay-go"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

// ensureBucketInputs checks and configures input specific information
func (r *ReconcileWebhookRelayForward) ensureBucketInputs(logger logr.Logger, instance *forwardv1.WebhookRelayForward, bucketSpec *forwardv1.BucketSpec) error {

	// If no inputs are defined, nothing to do
	if len(bucketSpec.Inputs) == 0 {
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

	// Create a list of desired inputs and then diff existing
	// ones against them to build a list of what inputs
	// we should create, update and which ones to delete
	desired := desiredInputs(bucketSpec, bucket)
	diff := getInputsDiff(bucket.Inputs, desired)

	var err error

	// Create inputs that need to be created
	for idx := range diff.create {
		logger.Info("creating input",
			"input_id", diff.create[idx].ID,
			"input_name", diff.create[idx].Name,
		)
		_, err = r.apiClient.client.CreateInput(diff.create[idx])
		if err != nil {
			logger.Error(err, "failed to create input")
		}
	}

	for idx := range diff.update {
		logger.Info("updating input",
			"input_id", diff.update[idx].ID,
			"input_name", diff.update[idx].Name,
		)
		_, err = r.apiClient.client.UpdateInput(diff.update[idx])
		if err != nil {
			logger.Error(err, "failed to update input",
				"input_id", diff.update[idx].ID,
			)
		}
	}

	for idx := range diff.delete {
		logger.Info("deleting input",
			"input_id", diff.delete[idx].ID,
			"input_name", diff.delete[idx].Name,
		)
		err = r.apiClient.client.DeleteInput(&webhookrelay.InputDeleteOptions{
			Bucket: diff.delete[idx].BucketID,
			Input:  diff.delete[idx].ID,
		})
		if err != nil {
			logger.Error(err, "failed to delete input",
				"input_id", diff.update[idx].ID,
			)
		}
	}

	return nil
}

func desiredInputs(bucketSpec *forwardv1.BucketSpec, bucket *webhookrelay.Bucket) []*webhookrelay.Input {

	var desired []*webhookrelay.Input

	for i := range bucketSpec.Inputs {
		desired = append(desired, inputSpecToInput(&bucketSpec.Inputs[i], bucket))
	}

	return desired
}

func inputSpecToInput(spec *forwardv1.InputSpec, bucket *webhookrelay.Bucket) *webhookrelay.Input {
	return &webhookrelay.Input{
		Name:       spec.Name,
		BucketID:   bucket.ID,
		FunctionID: spec.FunctionID,
		Headers:    spec.ResponseHeaders,
		StatusCode: spec.ResponseStatusCode,
		Body:       spec.ResponseBody,

		ResponseFromOutput: spec.ResponseFromOutput,
		CustomDomain:       spec.CustomDomain,
		PathPrefix:         spec.PathPrefix,
		Description:        spec.Description,
	}
}

func getInputsDiff(current, desired []*webhookrelay.Input) *inputsDiff {

	diff := &inputsDiff{}

	currentMap := make(map[string]*webhookrelay.Input)

	for i := range current {
		currentMap[current[i].Name] = current[i]
	}

	for i := range desired {
		currentInput, ok := currentMap[desired[i].Name]
		if !ok {
			diff.create = append(diff.create, desired[i])
			continue
		}
		if inputEqual(currentInput, desired[i]) {
			// Nothing to do
			continue
		}
		// Setting ID and adding to the update list
		desired[i].ID = currentInput.ID
		diff.update = append(diff.update, desired[i])
	}

	// TODO: check for inputs to delete, however this can be tricky and
	// dangerous as it's better to have unused inputs than delete an input
	// that's already being used by something and then have to manually update
	// 3rd party service with the new ID

	return diff
}

func inputEqual(current, desired *webhookrelay.Input) bool {

	return true
}

type inputsDiff struct {
	create []*webhookrelay.Input
	update []*webhookrelay.Input
	delete []*webhookrelay.Input
}
