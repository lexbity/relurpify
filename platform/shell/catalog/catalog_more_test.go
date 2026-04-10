package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEntryFromCommandSpecNormalizesDefaults(t *testing.T) {
	entry := EntryFromCommandSpec(CommandToolSpec{
		Name:        "  CLI-Example  ",
		Aliases:     []string{"One", "Two Words", "", "One"},
		Family:      "  Text Tools  ",
		Intent:      []string{" Transform ", "", "Inspect"},
		Description: "example",
		LongDescription: "long form",
		CommandTemplate: []string{"", "awk", " ", "$1"},
		DefaultArgs:     []string{"-n", ""},
		Tags:            []string{" Execute ", ""},
		Deprecated:      true,
		Replacement:     "  replacement-tool  ",
		ParameterSchema: ToolSchema{},
		OutputSchema: ToolSchema{
			Type: "array",
			Items: &ToolSchemaField{Type: "string"},
		},
		Examples: []ToolExample{{Query: "demo", Output: "ok"}},
	})

	require.Equal(t, "cli_example", entry.Name)
	require.Equal(t, []string{"one", "two_words", "one"}, entry.Aliases)
	require.Equal(t, "text_tools", entry.Family)
	require.Equal(t, []string{"transform", "inspect"}, entry.Intent)
	require.Equal(t, "example", entry.Description)
	require.Equal(t, "long form", entry.LongDescription)
	require.Equal(t, ToolSchema{Type: "object"}, entry.ParameterSchema)
	require.Equal(t, "array", entry.OutputSchema.Type)
	require.Equal(t, []string{"awk", "$1"}, entry.Preset.CommandTemplate)
	require.Equal(t, []string{"-n", ""}, entry.Preset.DefaultArgs)
	require.True(t, entry.Preset.AllowStdin)
	require.True(t, entry.Preset.SupportsWorkdir)
	require.Equal(t, []string{"execute"}, entry.Tags)
	require.True(t, entry.Deprecated)
	require.Equal(t, "replacement_tool", entry.Replacement)
	require.Len(t, entry.Examples, 1)
}

func TestEntryFromCommandSpecUsesCommandFallbackWhenTemplateMissing(t *testing.T) {
	entry := EntryFromCommandSpec(CommandToolSpec{
		Name:        "CLI-Fallback",
		Family:      "Archive",
		Description: "fallback",
		Command:     " tar ",
		ParameterSchema: ToolSchema{Type: "object"},
		OutputSchema:    ToolSchema{Type: "object"},
	})

	require.Equal(t, "cli_fallback", entry.Name)
	require.Equal(t, []string{"tar"}, entry.Preset.CommandTemplate)
	require.Equal(t, "archive", entry.Family)
	require.Equal(t, ToolSchema{Type: "object"}, entry.ParameterSchema)
	require.Equal(t, ToolSchema{Type: "object"}, entry.OutputSchema)
}

func TestRegisterRejectsConflictsAndNilCatalog(t *testing.T) {
	var nilCatalog *ToolCatalog
	err := nilCatalog.Register(ToolCatalogEntry{
		Name:            "cli_nil",
		ParameterSchema: ToolSchema{Type: "object"},
		OutputSchema:    ToolSchema{Type: "object"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "catalog missing")

	cat := NewToolCatalog()
	require.NoError(t, cat.Register(ToolCatalogEntry{
		Name:            "cli_first",
		Aliases:         []string{"shared-alias", "first"},
		ParameterSchema: ToolSchema{Type: "object"},
		OutputSchema:    ToolSchema{Type: "object"},
	}))

	err = cat.Register(ToolCatalogEntry{
		Name:            "cli_second",
		Aliases:         []string{"shared-alias"},
		ParameterSchema: ToolSchema{Type: "object"},
		OutputSchema:    ToolSchema{Type: "object"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "alias")

	err = cat.Register(ToolCatalogEntry{
		Name:            "cli-first",
		ParameterSchema: ToolSchema{Type: "object"},
		OutputSchema:    ToolSchema{Type: "object"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "already registered")
}

func TestValidateRejectsInvalidReplacementAndSchema(t *testing.T) {
	entry := ToolCatalogEntry{
		Name:        "cli_invalid",
		Deprecated:  true,
		Replacement: "   ",
		ParameterSchema: ToolSchema{
			Type: "object",
			Properties: map[string]ToolSchemaField{
				"mode": {
					Type:    "string",
					Enum:    []string{"fast", "safe"},
					Default: "slow",
				},
				"options": {
					Type: "object",
					Required: []string{"missing"},
					Properties: map[string]ToolSchemaField{
						"inner": {Type: "array"},
					},
				},
			},
			Required: []string{"mode", "missing"},
		},
		OutputSchema: ToolSchema{
			Type: "array",
		},
	}

	err := entry.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "replacement")
	require.Contains(t, err.Error(), "parameter_schema")
	require.Contains(t, err.Error(), "output_schema")

	schemaErr := ToolSchema{
		Type: "object",
		Properties: map[string]ToolSchemaField{
			"items": {
				Type: "array",
			},
		},
		Required: []string{"items"},
	}.Validate()
	require.Error(t, schemaErr)
	require.Contains(t, schemaErr.Error(), "items")
}
