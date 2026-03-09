package core

import "testing"

import "github.com/stretchr/testify/require"

func TestValidateValueAgainstSchemaAcceptsMatchingObject(t *testing.T) {
	err := ValidateValueAgainstSchema(map[string]interface{}{
		"path":  "README.md",
		"limit": 2.0,
	}, &Schema{
		Type: "object",
		Properties: map[string]*Schema{
			"path":  {Type: "string"},
			"limit": {Type: "integer"},
		},
		Required: []string{"path"},
	})
	require.NoError(t, err)
}

func TestValidateValueAgainstSchemaRejectsMissingRequiredField(t *testing.T) {
	err := ValidateValueAgainstSchema(map[string]interface{}{}, &Schema{
		Type:     "object",
		Required: []string{"path"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "required")
}
