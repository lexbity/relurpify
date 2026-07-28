package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitSchemaDocumentParsesHeaderAndBody(t *testing.T) {
	data := []byte(`# comment

schema: relurpify/tool/v1
name: read_file
`)

	decl, body, err := SplitSchemaDocument("tool.yaml", data)
	require.NoError(t, err)
	require.Equal(t, "relurpify/tool/v1", decl.Raw)
	require.Equal(t, schemaKindTool, decl.Kind)
	require.Equal(t, 1, decl.Version)
	require.Equal(t, 3, decl.Line)
	require.Equal(t, "name: read_file\n", string(body))
}

func TestSplitSchemaDocumentRejectsMissingSchema(t *testing.T) {
	_, _, err := SplitSchemaDocument("tool.yaml", []byte("name: read_file\n"))
	var schemaErr *SchemaError
	require.ErrorAs(t, err, &schemaErr)
	require.ErrorIs(t, err, ErrMissingSchemaDeclaration)
}

func TestSplitSchemaDocumentRejectsInvalidSchemaLine(t *testing.T) {
	_, _, err := SplitSchemaDocument("tool.yaml", []byte("kind: tool\n"))
	var schemaErr *SchemaError
	require.ErrorAs(t, err, &schemaErr)
	require.ErrorIs(t, err, ErrMissingSchemaDeclaration)
}

func TestSchemaRegistryRejectsUnknownAndUnsupported(t *testing.T) {
	reg := NewSchemaRegistry()

	require.NoError(t, reg.Register("custom/tool", 7))
	require.ErrorIs(t, reg.Require(SchemaDeclaration{Kind: "missing/tool", Version: 1, Line: 1}), ErrUnknownSchema)
	require.ErrorIs(t, reg.Require(SchemaDeclaration{Kind: "custom/tool", Version: 1, Line: 1}), ErrUnsupportedSchemaVersion)
	require.NoError(t, reg.Require(SchemaDeclaration{Kind: "custom/tool", Version: 7, Line: 1}))
}

func TestSchemaRegistryKnownKindsAreSorted(t *testing.T) {
	reg := NewSchemaRegistry()
	require.Equal(t, []string{
		"model/profile",
		"model/provider",
		"policy/ingestion",
		"policy/localtool",
		"policy/sandbox",
		"policy/shell",
		schemaKindTool,
		"workspace",
	}, reg.KnownKinds())
}
