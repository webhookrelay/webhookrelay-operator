package webhookrelayforward

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/webhookrelay/webhookrelay-go"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

func (r *ReconcileWebhookRelayForward) ensureBucketOutputs(logger logr.Logger, bucketSpec *forwardv1.BucketSpec) error {
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
	desired, conversionErr := desiredOutputs(bucketSpec, bucket)
	if conversionErr != nil {
		return conversionErr
	}
	diff := getOutputsDiff(bucket.Outputs, desired)

	var (
		err     error
		created *webhookrelay.Output
		updated *webhookrelay.Output
	)
	// Create inputs that need to be created
	for idx := range diff.create {
		logger.Info("creating output",
			"output_id", diff.create[idx].ID,
			"output_name", diff.create[idx].Name,
		)
		created, err = r.apiClient.client.CreateOutput(diff.create[idx])
		if err != nil {
			logger.Error(err, "failed to create output")
			continue
		}
		// updating cache
		r.apiClient.bucketsCache.AddOutput(created)
	}

	for idx := range diff.update {
		logger.Info("updating output",
			"output_id", diff.update[idx].ID,
			"output_name", diff.update[idx].Name,
		)
		updated, err = r.apiClient.client.UpdateOutput(diff.update[idx])
		if err != nil {
			logger.Error(err, "failed to update input",
				"input_id", diff.update[idx].ID,
			)
			continue
		}
		r.apiClient.bucketsCache.AddOutput(updated)
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

func desiredOutputs(bucketSpec *forwardv1.BucketSpec, bucket *webhookrelay.Bucket) ([]*webhookrelay.Output, error) {
	var desired []*webhookrelay.Output

	for i := range bucketSpec.Outputs {
		output, err := outputSpecToOutput(&bucketSpec.Outputs[i], bucket)
		if err != nil {
			return nil, fmt.Errorf("invalid configuration for output %q: %w", bucketSpec.Outputs[i].Name, err)
		}
		desired = append(desired, output)
	}

	return desired, nil
}

func outputSpecToOutput(spec *forwardv1.OutputSpec, bucket *webhookrelay.Bucket) (*webhookrelay.Output, error) {
	if err := validateOutputSpec(spec); err != nil {
		return nil, err
	}
	header := make(map[string][]string)

	if spec.OverrideHeaders != nil {
		for k, v := range spec.OverrideHeaders {
			header[k] = []string{v}
		}
	}

	output := &webhookrelay.Output{
		Name:               spec.Name,
		BucketID:           bucket.ID,
		FunctionID:         spec.EffectiveFunctionID(),
		ResponseFunctionID: spec.ResponseFunctionID,
		Headers:            header,
		Destination:        spec.Destination,
		Timeout:            spec.Timeout,
		Description:        spec.Description,
	}
	applyScalarOutputOptions(spec, output)
	if spec.Rules != nil && len(spec.Rules.Raw) > 0 {
		if err := json.Unmarshal(spec.Rules.Raw, &output.Rules); err != nil {
			return nil, fmt.Errorf("invalid rules: %w", err)
		}
	}
	var err error
	output.Durability, err = durabilityFromSpec(spec.Durability)
	if err != nil {
		return nil, fmt.Errorf("invalid durability: %w", err)
	}
	output.Throttle, err = throttleFromSpec(spec.Throttle)
	if err != nil {
		return nil, fmt.Errorf("invalid throttle: %w", err)
	}
	output.ReplayMissing, err = replayMissingFromSpec(spec.ReplayMissing)
	if err != nil {
		return nil, fmt.Errorf("invalid replayMissing: %w", err)
	}

	return output, nil
}

func validateOutputSpec(spec *forwardv1.OutputSpec) error {
	if spec.FunctionID != "" && spec.LegacyFunctionID != "" && spec.FunctionID != spec.LegacyFunctionID {
		return fmt.Errorf("functionId conflicts with deprecated function_id")
	}
	if spec.ReplayMissing != nil && spec.ReplayMissing.Enabled && (spec.Internal == nil || !*spec.Internal) {
		return fmt.Errorf("replayMissing requires internal: true")
	}
	return nil
}

func applyScalarOutputOptions(spec *forwardv1.OutputSpec, output *webhookrelay.Output) {
	if spec.Retries != nil {
		output.Retries = *spec.Retries
	}
	if spec.TLSVerification != nil {
		output.TLSVerification = *spec.TLSVerification
	}
	if spec.LockPath != nil {
		output.LockPath = *spec.LockPath
	}
	if spec.Disabled != nil {
		output.Disabled = *spec.Disabled
	}
	if spec.Internal != nil {
		output.Internal = *spec.Internal
	}
}

func outputsEqual(current, desired *webhookrelay.Output) bool {
	if current.Name != desired.Name {
		return false
	}
	if current.FunctionID != desired.FunctionID {
		return false
	}
	if current.ResponseFunctionID != desired.ResponseFunctionID {
		return false
	}
	if current.Destination != desired.Destination {
		return false
	}

	if !headersEqual(current.Headers, desired.Headers) {
		return false
	}

	if current.Internal != desired.Internal {
		return false
	}

	if current.LockPath != desired.LockPath {
		return false
	}

	if current.Disabled != desired.Disabled {
		return false
	}

	if current.Timeout != desired.Timeout {
		return false
	}
	if current.Retries != desired.Retries {
		return false
	}
	if current.TLSVerification != desired.TLSVerification {
		return false
	}
	if !reflect.DeepEqual(current.Rules, desired.Rules) {
		return false
	}
	if !reflect.DeepEqual(current.Durability, desired.Durability) {
		return false
	}
	if !reflect.DeepEqual(current.Throttle, desired.Throttle) {
		return false
	}
	if !reflect.DeepEqual(current.ReplayMissing, desired.ReplayMissing) {
		return false
	}

	if current.Description != desired.Description {
		return false
	}

	return true
}

func durabilityFromSpec(spec *forwardv1.DurabilitySpec) (*webhookrelay.DurabilityConfig, error) {
	if spec == nil {
		return nil, nil
	}
	config := &webhookrelay.DurabilityConfig{Enabled: spec.Enabled, Schedule: spec.Schedule}
	for _, delay := range spec.CustomDelays {
		parsed, err := parseDuration(delay)
		if err != nil {
			return nil, fmt.Errorf("custom delay: %w", err)
		}
		config.CustomDelays = append(config.CustomDelays, parsed)
	}
	if spec.Deadline != "" {
		parsed, err := parseDuration(spec.Deadline)
		if err != nil {
			return nil, fmt.Errorf("deadline: %w", err)
		}
		config.Deadline = parsed
	}
	if spec.HandoffAfter != "" {
		parsed, err := parseDuration(spec.HandoffAfter)
		if err != nil {
			return nil, fmt.Errorf("handoffAfter: %w", err)
		}
		config.HandoffAfter = parsed
	}
	return config, nil
}

func throttleFromSpec(spec *forwardv1.ThrottleSpec) (*webhookrelay.ThrottleConfig, error) {
	if spec == nil {
		return nil, nil
	}
	config := &webhookrelay.ThrottleConfig{
		Enabled:       spec.Enabled,
		Mode:          spec.Mode,
		Rate:          spec.Rate,
		Interval:      spec.Interval,
		MaxConcurrent: spec.MaxConcurrent,
		MaxQueueDepth: spec.MaxQueueDepth,
	}
	if spec.Deadline != "" {
		parsed, err := parseDuration(spec.Deadline)
		if err != nil {
			return nil, fmt.Errorf("deadline: %w", err)
		}
		config.Deadline = parsed
	}
	return config, nil
}

func replayMissingFromSpec(spec *forwardv1.ReplayMissingSpec) (*webhookrelay.ReplayMissingConfig, error) {
	if spec == nil {
		return nil, nil
	}
	config := &webhookrelay.ReplayMissingConfig{Enabled: spec.Enabled, Limit: spec.Limit}
	if spec.Lookback != "" {
		parsed, err := parseDuration(spec.Lookback)
		if err != nil {
			return nil, fmt.Errorf("lookback: %w", err)
		}
		config.Lookback = parsed
	}
	return config, nil
}

func parseDuration(value forwardv1.Duration) (time.Duration, error) {
	duration, err := time.ParseDuration(string(value))
	if err != nil {
		return 0, fmt.Errorf("%q is not a valid duration: %w", value, err)
	}
	return duration, nil
}

func headersEqual(current, desired map[string][]string) bool {
	currentNormalized := normalizeHeaders(current)
	desiredNormalized := normalizeHeaders(desired)
	if len(currentNormalized) != len(desiredNormalized) {
		return false
	}
	for key, desiredValues := range desiredNormalized {
		currentValues, ok := currentNormalized[key]
		if !ok || !sliceEqual(currentValues, desiredValues) {
			return false
		}
	}
	return true
}

func normalizeHeaders(headers map[string][]string) map[string][]string {
	normalized := make(map[string][]string, len(headers))
	for key, values := range headers {
		normalized[strings.ToLower(key)] = values
	}
	return normalized
}
