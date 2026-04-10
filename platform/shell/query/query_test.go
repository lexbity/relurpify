package query

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/platform/shell/catalog"
	shelltelemetry "github.com/lexcodex/relurpify/platform/shell/telemetry"
	"github.com/stretchr/testify/require"
)

func testCatalog(t *testing.T) *catalog.ToolCatalog {
	t.Helper()
	cat := catalog.NewToolCatalog()
	entries := []catalog.ToolCatalogEntry{
		catalog.EntryFromCommandSpec(catalog.CommandToolSpec{
			Name:        "cli_cargo",
			Family:      "build",
			Intent:      []string{"build", "rust"},
			Description: "Executes Rust cargo commands inside the workspace.",
			Command:     "cargo",
		}),
		catalog.EntryFromCommandSpec(catalog.CommandToolSpec{
			Name:        "cli_go",
			Family:      "build",
			Intent:      []string{"build", "go"},
			Description: "Executes Go commands inside the workspace.",
			Command:     "go",
		}),
		catalog.EntryFromCommandSpec(catalog.CommandToolSpec{
			Name:        "cli_git",
			Aliases:     []string{"git"},
			Family:      "fileops",
			Intent:      []string{"inspect", "repository"},
			Description: "Runs git commands.",
			Command:     "git",
		}),
		{
			Name:        "cli_search",
			Family:      "text",
			Intent:      []string{"search", "inspect"},
			Aliases:     []string{"grep"},
			Description: "Searches files with structured path and pattern inputs.",
			ParameterSchema: catalog.ToolSchema{
				Type: "object",
				Properties: map[string]catalog.ToolSchemaField{
					"path":    {Type: "string"},
					"pattern": {Type: "string"},
				},
				Required: []string{"path", "pattern"},
			},
			OutputSchema: catalog.ToolSchema{Type: "object"},
			Preset: catalog.ToolPreset{
				CommandTemplate: []string{"grep"},
				AllowStdin:      true,
				SupportsWorkdir: true,
			},
		},
	}
	for _, entry := range entries {
		require.NoError(t, cat.Register(entry))
	}
	return cat
}

func TestParseDiscoveryQueryRejectsUnknownFields(t *testing.T) {
	_, err := ParseDiscoveryQuery(map[string]any{
		"tool_name": "cargo",
		"nope":      true,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown discovery field")
}

func TestParseDiscoveryQueryRejectsOversizedResultSets(t *testing.T) {
	_, err := ParseDiscoveryQuery(map[string]any{
		"tool_name":   "cargo",
		"max_results": 100,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "at most")
}

func TestDiscoverySearchRanksWorkspaceHintsAndCanonicalNames(t *testing.T) {
	engine := NewEngine(testCatalog(t))

	result, err := engine.Search(DiscoveryQuery{
		Intent:           []string{"rust"},
		WorkspaceContext: WorkspaceHints{HasCargoToml: true},
		MaxResults:       5,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Matches)
	require.Equal(t, "cli_cargo", result.Matches[0].Entry.Name)
	require.Contains(t, result.NormalizedQuery, "rust")
	require.Greater(t, result.FamilySummary["build"], 0)

	repeat, err := engine.Search(DiscoveryQuery{
		Intent:           []string{"rust"},
		WorkspaceContext: WorkspaceHints{HasCargoToml: true},
		MaxResults:       5,
	})
	require.NoError(t, err)
	require.Equal(t, result.Matches[0].Entry.Name, repeat.Matches[0].Entry.Name)
	require.Equal(t, result.Matches[0].Score, repeat.Matches[0].Score)
}

func TestDiscoverySearchReturnsAliasMatches(t *testing.T) {
	engine := NewEngine(testCatalog(t))

	result, err := engine.Search(DiscoveryQuery{
		Aliases:    []string{"grep"},
		MaxResults: 5,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Matches)
	require.Equal(t, "cli_search", result.Matches[0].Entry.Name)
	require.Contains(t, result.Matches[0].Entry.Aliases, "grep")
}

func TestParseInstantiationQueryAndValidateRequiredParams(t *testing.T) {
	raw := map[string]any{
		"tool_name": "cli_search",
		"arguments": map[string]any{
			"path":    "src",
			"pattern": "hello",
			"args":    []any{"--line-number"},
		},
		"workspace_context": map[string]any{
			"is_git_repo": true,
		},
	}
	q, err := ParseInstantiationQuery(raw)
	require.NoError(t, err)
	require.Equal(t, "cli_search", q.ToolName)
	require.True(t, q.WorkspaceContext.IsGitRepo)

	engine := NewEngine(testCatalog(t))
	result, err := engine.Instantiate(q)
	require.NoError(t, err)
	require.Equal(t, "cli_search", result.Match.Entry.Name)
	require.Equal(t, []string{"grep", "--line-number"}, result.Request.Args)
	require.Equal(t, "src", result.StructuredArgs["path"])
	require.Equal(t, "hello", result.StructuredArgs["pattern"])
	require.Equal(t, "grep", result.Preset.Command)
	require.True(t, result.Request.Input == "")
}

func TestInstantiationRejectsUnknownToolAndMissingParams(t *testing.T) {
	engine := NewEngine(testCatalog(t))

	_, err := engine.Instantiate(InstantiationQuery{ToolName: "missing"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "tool not found")

	_, err = engine.Instantiate(InstantiationQuery{
		ToolName:  "cli_search",
		Arguments: map[string]any{"path": "src"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing required parameter")
}

func TestInstantiationRejectsAmbiguousFamilyOnlyRequests(t *testing.T) {
	engine := NewEngine(testCatalog(t))

	_, err := engine.Instantiate(InstantiationQuery{Family: "build"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "ambiguous")
}

func TestQueryToolsReturnStructuredDiscoveryAndInstantiationData(t *testing.T) {
	tools := Tools(testCatalog(t))
	require.Len(t, tools, 2)

	var discovery, instantiation core.Tool
	for _, tool := range tools {
		switch tool.Name() {
		case discoveryToolName:
			discovery = tool
		case instantiationToolName:
			instantiation = tool
		}
	}
	require.NotNil(t, discovery)
	require.NotNil(t, instantiation)

	discoveryResult, err := discovery.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"aliases":     []any{"grep"},
		"max_results": 3,
	})
	require.NoError(t, err)
	require.True(t, discoveryResult.Success)
	require.NotEmpty(t, discoveryResult.Data["matches"])
	require.Contains(t, discoveryResult.Metadata, "query_type")

	instantiationResult, err := instantiation.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"tool_name": "cli_search",
		"arguments": map[string]any{
			"path":    "src",
			"pattern": "hello",
			"args":    []any{"--line-number"},
		},
	})
	require.NoError(t, err)
	require.True(t, instantiationResult.Success)
	require.NotEmpty(t, instantiationResult.Data["request"])
	require.NotEmpty(t, instantiationResult.Data["preset"])
	require.NotEmpty(t, instantiationResult.Data["tool"])
}

type recordingTelemetry struct {
	events []shelltelemetry.Event
}

func (r *recordingTelemetry) Emit(event shelltelemetry.Event) {
	r.events = append(r.events, event)
}

func TestQueryToolsEmitTelemetryForSearchAliasAndDeprecatedUsage(t *testing.T) {
	cat := testCatalog(t)
	require.NoError(t, cat.Register(catalog.EntryFromCommandSpec(catalog.CommandToolSpec{
		Name:        "cli_old_git",
		Aliases:     []string{"old-git"},
		Family:      "fileops",
		Intent:      []string{"repository"},
		Description: "Deprecated git wrapper.",
		Command:     "git",
		Deprecated:  true,
		Replacement: "cli_git",
	})))

	telemetry := &recordingTelemetry{}
	tools := ToolsWithTelemetry(cat, telemetry)
	var discovery, instantiation core.Tool
	for _, tool := range tools {
		switch tool.Name() {
		case discoveryToolName:
			discovery = tool
		case instantiationToolName:
			instantiation = tool
		}
	}
	require.NotNil(t, discovery)
	require.NotNil(t, instantiation)

	_, err := discovery.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"aliases": []any{"grep"},
	})
	require.NoError(t, err)

	_, err = instantiation.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"aliases":          []any{"old-git"},
		"allow_deprecated": true,
	})
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(telemetry.events), 5)
	require.Equal(t, "tool_call", telemetry.events[0].Type)
	require.Contains(t, telemetry.events[0].Message, "discovery")

	var aliasResolved bool
	var deprecatedSeen bool
	for _, event := range telemetry.events {
		if event.Message == "shell alias resolved" {
			aliasResolved = true
		}
		if event.Message == "shell instantiation query completed" {
			if dep, ok := event.Metadata["deprecated"].(bool); ok && dep {
				deprecatedSeen = true
			}
		}
	}
	require.True(t, aliasResolved)
	require.True(t, deprecatedSeen)
}
