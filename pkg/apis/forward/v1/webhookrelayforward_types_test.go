package v1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputFunctionIDMigration(t *testing.T) {
	var legacy OutputSpec
	require.NoError(t, json.Unmarshal([]byte(`{"function_id":"legacy-function"}`), &legacy))
	assert.Equal(t, "legacy-function", legacy.EffectiveFunctionID())

	var migrated OutputSpec
	require.NoError(t, json.Unmarshal([]byte(`{"functionId":"new-function","function_id":"legacy-function"}`), &migrated))
	assert.Equal(t, "new-function", migrated.EffectiveFunctionID())

	encoded, err := json.Marshal(OutputSpec{FunctionID: "new-function"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"functionId":"new-function","destination":""}`, string(encoded))
}
