package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFromMapConvertsNestedObjectSchema(t *testing.T) {
	s, err := FromMap(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
			"filters": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required": []any{"query"},
	})
	require.NoError(t, err)
	require.NotNil(t, s)
	require.Equal(t, "object", s.Type)
	require.Equal(t, "string", s.Properties["query"].Type)
	require.Equal(t, "array", s.Properties["filters"].Type)
	require.Equal(t, "string", s.Properties["filters"].Items.Type)
	require.Equal(t, []string{"query"}, s.Required)
}
