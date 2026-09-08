package webhookrelayforward

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/webhookrelay/webhookrelay-go"

	forwardv1 "github.com/webhookrelay/webhookrelay-operator/pkg/apis/forward/v1"
)

func TestInputSpecToInputUsesAPIStatusDefault(t *testing.T) {
	input := inputSpecToInput(
		&forwardv1.InputSpec{Name: "input"},
		&webhookrelay.Bucket{ID: "bucket"},
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
