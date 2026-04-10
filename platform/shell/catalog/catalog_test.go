package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeName(t *testing.T) {
	require.Equal(t, "cli_git", NormalizeName("  CLI-Git  "))
	require.Equal(t, "shell_binding", NormalizeName("shell.binding"))
	require.Equal(t, "tool_name", NormalizeName("tool name"))
}

func TestAliasLookupResolvesCanonicalName(t *testing.T) {
	cat := NewToolCatalog()
	require.NoError(t, cat.Register(ToolCatalogEntry{
		Name:        "CLI-Mkdir",
		Aliases:     []string{"mkdir", "make dir"},
		Family:      "fileops",
		Description: "Creates directories.",
		ParameterSchema: ToolSchema{
			Type: "object",
		},
		OutputSchema: ToolSchema{
			Type: "object",
		},
	}))

	entry, ok := cat.Lookup("mkdir")
	require.True(t, ok)
	require.Equal(t, "cli_mkdir", entry.Name)

	entry, ok = cat.Lookup("CLI-MKDIR")
	require.True(t, ok)
	require.Equal(t, "cli_mkdir", entry.Name)
}

func TestSchemaValidationReportsFieldPaths(t *testing.T) {
	schema := ToolSchema{
		Type: "object",
		Properties: map[string]ToolSchemaField{
			"query": {
				Type: "string",
			},
			"filters": {
				Type: "array",
			},
			"options": {
				Type: "object",
				Properties: map[string]ToolSchemaField{
					"mode": {
						Type:  "string",
						Enum:  []string{"fast", "safe"},
						Default: "slow",
					},
				},
				Required: []string{"missing"},
			},
		},
		Required: []string{"query", "missing"},
	}

	err := schema.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "schema.required[1]")
	require.Contains(t, err.Error(), "filters.items")
	require.Contains(t, err.Error(), "options.required[0]")
	require.Contains(t, err.Error(), "options.properties.mode.default")
}

func TestDeprecatedEntriesRemainDiscoverable(t *testing.T) {
	cat := NewToolCatalog()
	require.NoError(t, cat.Register(ToolCatalogEntry{
		Name:        "cli_old_git",
		Aliases:     []string{"old-git"},
		Family:      "git",
		Description: "Deprecated git wrapper.",
		Deprecated:  true,
		Replacement: "cli_git",
		ParameterSchema: ToolSchema{
			Type: "object",
		},
		OutputSchema: ToolSchema{
			Type: "object",
		},
	}))

	entry, ok := cat.Lookup("old-git")
	require.True(t, ok)
	require.True(t, entry.Deprecated)
	require.Equal(t, "cli_git", NormalizeName(entry.Replacement))
}

func TestCatalogOrderingIsDeterministic(t *testing.T) {
	cat := NewToolCatalog()
	require.NoError(t, cat.Register(ToolCatalogEntry{
		Name: "cli_zeta",
		ParameterSchema: ToolSchema{Type: "object"},
		OutputSchema:    ToolSchema{Type: "object"},
	}))
	require.NoError(t, cat.Register(ToolCatalogEntry{
		Name: "cli_alpha",
		ParameterSchema: ToolSchema{Type: "object"},
		OutputSchema:    ToolSchema{Type: "object"},
	}))
	require.NoError(t, cat.Register(ToolCatalogEntry{
		Name: "cli_middle",
		ParameterSchema: ToolSchema{Type: "object"},
		OutputSchema:    ToolSchema{Type: "object"},
	}))

	entries := cat.List()
	require.Equal(t, []string{"cli_alpha", "cli_middle", "cli_zeta"}, []string{entries[0].Name, entries[1].Name, entries[2].Name})
}

func TestCatalogExampleMetadataIsPreserved(t *testing.T) {
	cat := NewToolCatalog()
	require.NoError(t, cat.Register(ToolCatalogEntry{
		Name:        "cli_example",
		Family:      "demo",
		Description: "Example entry.",
		Examples: []ToolExample{
			{
				Query:  "show me the example",
				Input:  map[string]any{"pattern": "hello"},
				Output: "matched output",
			},
		},
		ParameterSchema: ToolSchema{Type: "object"},
		OutputSchema:    ToolSchema{Type: "object"},
	}))

	entry, ok := cat.Lookup("cli_example")
	require.True(t, ok)
	require.Len(t, entry.Examples, 1)
	require.Equal(t, "show me the example", entry.Examples[0].Query)
	require.Equal(t, "matched output", entry.Examples[0].Output)
}
