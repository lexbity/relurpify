package cfgload

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeWithSchemaRejectsAnchorsAndAliases(t *testing.T) {
	data := []byte(`schema: relurpify/tool/v1
name: &tool_name read_file
copy: *tool_name
`)

	var out map[string]any
	_, err := DecodeWithSchema("tool.yaml", data, nil, &out)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrYAMLAnchorAlias)
	require.Contains(t, err.Error(), "tool.yaml")
}

func TestDecodeWithSchemaDecodesStrictly(t *testing.T) {
	data := []byte(`schema: relurpify/tool/v1
name: read_file
enabled: true
`)

	var out struct {
		Name    string `yaml:"name"`
		Enabled bool   `yaml:"enabled"`
	}
	decl, err := DecodeWithSchema("tool.yaml", data, nil, &out)
	require.NoError(t, err)
	require.Equal(t, "tool", decl.Kind)
	require.Equal(t, "read_file", out.Name)
	require.True(t, out.Enabled)
}

func TestDecodeWithSchemaRejectsUnknownSchema(t *testing.T) {
	data := []byte(`schema: relurpify/unknown/v1
name: test
`)

	var out map[string]any
	_, err := DecodeWithSchema("unknown.yaml", data, nil, &out)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnknownSchema)
}

func TestDecodeWithSchemaRejectsUnsupportedVersion(t *testing.T) {
	reg := NewSchemaRegistry()

	data := []byte(`schema: relurpify/tool/v2
name: test
`)

	var out map[string]any
	_, err := DecodeWithSchema("tool.yaml", data, reg, &out)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsupportedSchemaVersion)
}
