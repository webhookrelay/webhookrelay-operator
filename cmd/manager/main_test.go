package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfiguredNamespaces(t *testing.T) {
	namespaces, err := configuredNamespaces("tenant-b, tenant-a,tenant-a")
	require.NoError(t, err)
	assert.Equal(t, []string{"tenant-a", "tenant-b"}, namespaceNames(namespaces))
}

func TestConfiguredNamespacesRejectsEmptyOrInvalidValues(t *testing.T) {
	_, err := configuredNamespaces("")
	require.ErrorContains(t, err, "at least one namespace")

	_, err = configuredNamespaces("Invalid_Namespace")
	require.ErrorContains(t, err, "is invalid")
}
