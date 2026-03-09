package core

import "testing"

import "github.com/stretchr/testify/require"

func TestRuntimeSafetySpecValidateRejectsNegativeValues(t *testing.T) {
	err := (RuntimeSafetySpec{MaxCallsPerCapability: -1}).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be >= 0")

	err = (RuntimeSafetySpec{MaxNetworkRequestsSession: -1}).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be >= 0")
}

func TestRedactMetadataMapRedactsSensitiveKeysAndValues(t *testing.T) {
	redacted := RedactMetadataMap(map[string]interface{}{
		"token":         "abc",
		"authorization": "Bearer secret",
		"nested": map[string]interface{}{
			"cookie": "session=123",
			"ok":     "value",
		},
		"plain": "hello",
	})

	require.Equal(t, "[REDACTED]", redacted["token"])
	require.Equal(t, "[REDACTED]", redacted["authorization"])
	nested := redacted["nested"].(map[string]interface{})
	require.Equal(t, "[REDACTED]", nested["cookie"])
	require.Equal(t, "value", nested["ok"])
	require.Equal(t, "hello", redacted["plain"])
}
