package webhookrelayforward

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/webhookrelay/webhookrelay-go"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

func TestInputSpecToInputUsesAPIStatusDefault(t *testing.T) {
	input := inputSpecToInput(
		&forwardv1.InputSpec{Name: testInputName},
		&webhookrelay.Bucket{ID: testBucketID},
	)

	assert.Equal(t, 200, input.StatusCode)
}

func TestInputEqualDetectsHeaderChanges(t *testing.T) {
	current := &webhookrelay.Input{Headers: map[string][]string{testHeaderName: {testHeaderBefore}}}
	desired := &webhookrelay.Input{Headers: map[string][]string{testHeaderLowerName: {testHeaderAfter}}}

	assert.False(t, inputEqual(current, desired))
	desired.Headers[testHeaderLowerName] = []string{testHeaderBefore}
	assert.True(t, inputEqual(current, desired))
}

func TestInputSpecToInputMapsTLSAndPathControls(t *testing.T) {
	input := inputSpecToInput(
		&forwardv1.InputSpec{
			Name:            testInputName,
			StripPathPrefix: true,
			TLSVersion:      "1.2",
			LegacyTLS:       true,
		},
		&webhookrelay.Bucket{ID: testBucketID},
	)

	assert.True(t, input.StripPathPrefix)
	assert.Equal(t, "1.2", input.TLSVersion)
	assert.True(t, input.LegacyTLS)
}
