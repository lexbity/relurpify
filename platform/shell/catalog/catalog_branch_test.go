package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaValidationErrorFormatting(t *testing.T) {
	var nilErr *SchemaValidationError
	require.Equal(t, "", nilErr.Error())
	require.Equal(t, "schema validation failed", (&SchemaValidationError{}).Error())
	require.Equal(t, "missing", (&SchemaValidationError{Issues: []SchemaIssue{{Message: "missing"}}}).Error())
	require.Equal(t, "schema.type: missing type", (&SchemaValidationError{Issues: []SchemaIssue{{Path: "schema.type", Message: "missing type"}}}).Error())
}

func TestToolCatalogLookupAndRegisterBranches(t *testing.T) {
	var nilCatalog *ToolCatalog
	_, ok := nilCatalog.Lookup("")
	require.False(t, ok)

	cat := NewToolCatalog()
	require.Error(t, cat.Register(ToolCatalogEntry{}))

	entry := ToolCatalogEntry{
		Name:            "CLI-Alpha",
		Aliases:         []string{"alpha", "cli-alpha"},
		ParameterSchema: ToolSchema{Type: "object"},
		OutputSchema:    ToolSchema{Type: "object"},
	}
	require.NoError(t, cat.Register(entry))

	got, ok := cat.Lookup("cli alpha")
	require.True(t, ok)
	require.Equal(t, "cli_alpha", got.Name)

	duplicateAlias := ToolCatalogEntry{
		Name:            "CLI-Beta",
		Aliases:         []string{"alpha"},
		ParameterSchema: ToolSchema{Type: "object"},
		OutputSchema:    ToolSchema{Type: "object"},
	}
	require.Error(t, cat.Register(duplicateAlias))
	require.Contains(t, cat.List()[0].Name, "cli_alpha")
}

func TestSchemaValidationBranches(t *testing.T) {
	invalid := ToolSchema{
		Type: "object",
		Properties: map[string]ToolSchemaField{
			"mode": {
				Type:    "string",
				Enum:    []string{"fast", "safe"},
				Default: "slow",
			},
			"items": {
				Type: "array",
			},
			"nested": {
				Type:     "object",
				Required: []string{""},
				Properties: map[string]ToolSchemaField{
					"child": {Type: ""},
				},
			},
		},
		Required: []string{"missing"},
	}

	err := invalid.ValidatePath("schema")
	require.Error(t, err)
	require.Contains(t, err.Error(), "schema.required[0]")
	require.Contains(t, err.Error(), "schema.properties.mode.default")
	require.Contains(t, err.Error(), "schema.properties.items.items")
	require.Contains(t, err.Error(), "schema.properties.nested.required[0]")
	require.Contains(t, err.Error(), "schema.properties.nested.properties.child.type")

	require.Error(t, ToolSchema{Type: "array"}.Validate())
	require.Error(t, ToolSchema{}.Validate())
}
