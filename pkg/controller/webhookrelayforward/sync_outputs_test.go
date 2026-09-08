package webhookrelayforward

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/webhookrelay/webhookrelay-go"
)

const (
	testHeaderName      = "X-Test"
	testHeaderLowerName = "x-test"
	testHeaderBefore    = "before"
	testHeaderAfter     = "after"
	testFirstHeader     = "X-First"
	testFirstValue      = "one"
)

func TestOutputsEqualDetectsHeaderChanges(t *testing.T) {
	current := &webhookrelay.Output{Headers: map[string][]string{testHeaderName: {testHeaderBefore}}}
	desired := &webhookrelay.Output{Headers: map[string][]string{testHeaderLowerName: {testHeaderAfter}}}

	assert.False(t, outputsEqual(current, desired))
	desired.Headers[testHeaderLowerName] = []string{testHeaderBefore}
	assert.True(t, outputsEqual(current, desired))
}

func TestHeadersEqualDetectsAddedAndRemovedHeaders(t *testing.T) {
	assert.False(t, headersEqual(
		map[string][]string{testFirstHeader: {testFirstValue}},
		map[string][]string{testFirstHeader: {testFirstValue}, "X-Second": {"two"}},
	))
	assert.True(t, headersEqual(nil, map[string][]string{}))
}
