package query

import (
	"testing"

	"github.com/lexcodex/relurpify/platform/shell/catalog"
	"github.com/stretchr/testify/require"
)

func TestQueryHelperBranches(t *testing.T) {
	b, err := asBool(true)
	require.NoError(t, err)
	require.True(t, b)
	b, err = asBool("true")
	require.NoError(t, err)
	require.True(t, b)
	_, err = asBool("maybe")
	require.Error(t, err)
	_, err = asBool(1)
	require.Error(t, err)
	_, err = asBool(nil)
	require.Error(t, err)

	n, err := asInt(int(7))
	require.NoError(t, err)
	require.Equal(t, 7, n)
	n, err = asInt(int32(8))
	require.NoError(t, err)
	require.Equal(t, 8, n)
	n, err = asInt(int64(9))
	require.NoError(t, err)
	require.Equal(t, 9, n)
	n, err = asInt(float64(10))
	require.NoError(t, err)
	require.Equal(t, 10, n)
	_, err = asInt(map[string]any{})
	require.Error(t, err)

	hints, err := parseWorkspaceHints(map[string]any{
		"has_cargo_toml":     true,
		"has_go_mod":         true,
		"has_package_json":   true,
		"has_python_files":   true,
		"has_notebook_files": false,
		"is_git_repo":        true,
		"language":           "Rust",
		"project_type":       "Web",
	})
	require.NoError(t, err)
	require.True(t, hints.HasCargoToml)
	require.True(t, hints.HasGoMod)
	require.True(t, hints.HasPackageJSON)
	require.True(t, hints.HasPythonFiles)
	require.True(t, hints.IsGitRepo)
	require.Equal(t, "Rust", hints.Language)
	require.Equal(t, "Web", hints.ProjectType)
	emptyHints, err := parseWorkspaceHints(nil)
	require.NoError(t, err)
	require.Empty(t, emptyHints)
	_, err = parseWorkspaceHints([]string{"bad"})
	require.Error(t, err)
	_, err = parseWorkspaceHints(map[string]any{"unknown": true})
	require.Error(t, err)

	entry := catalog.EntryFromCommandSpec(catalog.CommandToolSpec{
		Name:            "cli_match",
		Family:          "text",
		Aliases:         []string{"grep"},
		Intent:          []string{"search", "inspect", "rust", "go", "node", "python", "repository", "http"},
		Description:     "Search and inspect text files.",
		LongDescription: "Find content quickly.",
		Tags:            []string{"search"},
		Examples: []catalog.ToolExample{{
			Query:  "find text",
			Input:  map[string]any{"path": "src"},
			Output: "matched",
		}},
		ParameterSchema: catalog.ToolSchema{
			Type: "object",
			Properties: map[string]catalog.ToolSchemaField{
				"path": {Type: "string"},
			},
			Required: []string{"path"},
		},
		OutputSchema: catalog.ToolSchema{Type: "object"},
	})

	require.True(t, matchesKeyword(entry, "search"))
	require.True(t, matchesKeyword(entry, "matched"))
	require.False(t, matchesKeyword(entry, "missing"))
	require.True(t, prefersOutput(entry, "text"))
	require.False(t, prefersOutput(entry, "json"))
	require.True(t, hasParameter(entry, "path"))
	require.False(t, hasParameter(entry, "missing"))
	require.NoError(t, validateInstantiationArgs(entry, map[string]any{"path": "src"}))
	require.Error(t, validateInstantiationArgs(entry, map[string]any{"path": "src", "bad": true}))
	reasons := []string{}
	require.Greater(t, workspaceBias(entry, WorkspaceHints{
		HasCargoToml:   true,
		HasGoMod:       true,
		HasPackageJSON: true,
		HasPythonFiles: true,
		IsGitRepo:      true,
		Language:       "rust",
		ProjectType:    "web",
	}, &reasons), 0)
	require.NotEmpty(t, reasons)

	require.True(t, containsNormalized([]string{"two-words"}, "two words"))
	require.False(t, containsNormalized([]string{"one"}, "missing"))
	require.True(t, contains([]string{"one", "two"}, "TWO"))

	_, err = parseWorkspaceHints(map[string]any{"has_cargo_toml": "maybe"})
	require.Error(t, err)
}
