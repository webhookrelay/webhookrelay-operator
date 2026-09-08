package webhookrelayforward

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/webhookrelay/webhookrelay-go"
	"k8s.io/apimachinery/pkg/runtime"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

const (
	testHeaderName      = "X-Test"
	testHeaderLowerName = "x-test"
	testHeaderBefore    = "before"
	testHeaderAfter     = "after"
	testFirstHeader     = "X-First"
	testFirstValue      = "one"
	testBucketID        = "bucket"
	testInputName       = "input"
	testOutputName      = "output"
	testDestination     = "https://example.com/webhooks"
	testRequestFunction = "request-function"
)

func TestOutputsEqualDetectsHeaderChanges(t *testing.T) {
	current := &webhookrelay.Output{Headers: map[string][]string{testHeaderName: {testHeaderBefore}}}
	desired := &webhookrelay.Output{Headers: map[string][]string{testHeaderLowerName: {testHeaderAfter}}}

	assert.False(t, outputsEqual(current, desired))
	desired.Headers[testHeaderLowerName] = []string{testHeaderBefore}
	assert.True(t, outputsEqual(current, desired))
}

func TestOutputSpecToOutputMapsDeliveryControls(t *testing.T) {
	retries := -1
	tlsVerification := true
	internal := true
	output, err := outputSpecToOutput(&forwardv1.OutputSpec{
		Name:               testOutputName,
		FunctionID:         testRequestFunction,
		LegacyFunctionID:   testRequestFunction,
		ResponseFunctionID: "response-function",
		Destination:        testDestination,
		Internal:           &internal,
		Retries:            &retries,
		TLSVerification:    &tlsVerification,
		Rules:              &runtime.RawExtension{Raw: []byte(`{"match":{"type":"value","value":"push","parameter":{"source":"header","name":"X-Event"}}}`)},
		Durability: &forwardv1.DurabilitySpec{
			Enabled:      true,
			Schedule:     "long",
			Deadline:     "720h",
			HandoffAfter: "15m",
		},
		Throttle: &forwardv1.ThrottleSpec{
			Enabled:       true,
			Mode:          "concurrency",
			MaxConcurrent: 5,
			MaxQueueDepth: 1000,
			Deadline:      "24h",
		},
		ReplayMissing: &forwardv1.ReplayMissingSpec{
			Enabled:  true,
			Lookback: "30m",
			Limit:    250,
		},
	}, &webhookrelay.Bucket{ID: testBucketID})
	require.NoError(t, err)

	assert.Equal(t, testRequestFunction, output.FunctionID)
	assert.Equal(t, "response-function", output.ResponseFunctionID)
	assert.Equal(t, -1, output.Retries)
	assert.True(t, output.TLSVerification)
	require.NotNil(t, output.Rules)
	assert.Equal(t, "push", output.Rules.Match.Value)
	require.NotNil(t, output.Durability)
	assert.Equal(t, 30*24*time.Hour, output.Durability.Deadline)
	require.NotNil(t, output.Throttle)
	assert.Equal(t, 24*time.Hour, output.Throttle.Deadline)
	require.NotNil(t, output.ReplayMissing)
	assert.Equal(t, 30*time.Minute, output.ReplayMissing.Lookback)
	assert.Equal(t, 250, output.ReplayMissing.Limit)
}

func TestOutputSpecToOutputRejectsInvalidDuration(t *testing.T) {
	_, err := outputSpecToOutput(&forwardv1.OutputSpec{
		Name:        testOutputName,
		Destination: testDestination,
		Durability:  &forwardv1.DurabilitySpec{Enabled: true, Deadline: "tomorrow"},
	}, &webhookrelay.Bucket{ID: testBucketID})

	require.ErrorContains(t, err, "not a valid duration")
}

func TestOutputSpecToOutputRejectsConflictingFunctionIDs(t *testing.T) {
	_, err := outputSpecToOutput(&forwardv1.OutputSpec{
		Name:             testOutputName,
		Destination:      testDestination,
		FunctionID:       "new-function",
		LegacyFunctionID: "legacy-function",
	}, &webhookrelay.Bucket{ID: testBucketID})

	require.ErrorContains(t, err, "functionId conflicts")
}

func TestOutputSpecToOutputRequiresInternalReplay(t *testing.T) {
	_, err := outputSpecToOutput(&forwardv1.OutputSpec{
		Name:          testOutputName,
		Destination:   testDestination,
		ReplayMissing: &forwardv1.ReplayMissingSpec{Enabled: true},
	}, &webhookrelay.Bucket{ID: testBucketID})

	require.ErrorContains(t, err, "requires internal")
}

func TestHeadersEqualDetectsAddedAndRemovedHeaders(t *testing.T) {
	assert.False(t, headersEqual(
		map[string][]string{testFirstHeader: {testFirstValue}},
		map[string][]string{testFirstHeader: {testFirstValue}, "X-Second": {"two"}},
	))
	assert.True(t, headersEqual(nil, map[string][]string{}))
}
