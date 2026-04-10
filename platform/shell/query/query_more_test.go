package query

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/lexcodex/relurpify/platform/shell/catalog"
	"github.com/lexcodex/relurpify/platform/shell/execute"
	shelltelemetry "github.com/lexcodex/relurpify/platform/shell/telemetry"
	"github.com/stretchr/testify/require"
)

type queryTelemetry struct {
	events []shelltelemetry.Event
}

func (q *queryTelemetry) Emit(event shelltelemetry.Event) {
	q.events = append(q.events, event)
}

func richSearchEntry() catalog.ToolCatalogEntry {
	return catalog.EntryFromCommandSpec(catalog.CommandToolSpec{
		Name:            "cli_search",
		Aliases:         []string{"grep"},
		Family:          "text",
		Intent:          []string{"search", "inspect"},
		Description:     "Searches files with structured path and pattern inputs.",
		LongDescription: "Fast structured search for source trees.",
		Command:         "grep",
		DefaultArgs:     []string{"--line-number"},
		ParameterSchema: catalog.ToolSchema{
			Type: "object",
			Properties: map[string]catalog.ToolSchemaField{
				"path":    {Type: "string"},
				"pattern": {Type: "string"},
			},
			Required: []string{"path", "pattern"},
		},
		OutputSchema: catalog.ToolSchema{Type: "object"},
		Examples: []catalog.ToolExample{
			{
				Query:  "grep pattern",
				Input:  map[string]any{"path": "src"},
				Output: "matched text",
			},
		},
	})
}

func TestQueryHelperCoercionsAndNormalization(t *testing.T) {
	s, err := asString("hello")
	require.NoError(t, err)
	require.Equal(t, "hello", s)
	_, err = asString(nil)
	require.Error(t, err)

	slice, err := asStringSlice([]string{"a", "b"})
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, slice)
	slice, err = asStringSlice([]any{"c", 3})
	require.Error(t, err)
	require.Nil(t, slice)
	slice, err = asStringSlice("single")
	require.NoError(t, err)
	require.Equal(t, []string{"single"}, slice)
	slice, err = asStringSlice("")
	require.NoError(t, err)
	require.Nil(t, slice)

	m, err := asStringMap(nil)
	require.NoError(t, err)
	require.Empty(t, m)
	m, err = asStringMap(map[string]any{"k": "v"})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"k": "v"}, m)
	_, err = asStringMap([]string{"bad"})
	require.Error(t, err)

	b, err := asBool(true)
	require.NoError(t, err)
	require.True(t, b)
	b, err = asBool("false")
	require.NoError(t, err)
	require.False(t, b)
	_, err = asBool(1)
	require.Error(t, err)

	n, err := asInt(int64(7))
	require.NoError(t, err)
	require.Equal(t, 7, n)
	n, err = asInt("12")
	require.NoError(t, err)
	require.Equal(t, 12, n)
	_, err = asInt(map[string]any{})
	require.Error(t, err)

	hints, err := parseWorkspaceHints(map[string]any{
		"has_cargo_toml":     true,
		"has_go_mod":         "true",
		"has_package_json":   false,
		"has_python_files":   true,
		"has_notebook_files": false,
		"is_git_repo":        true,
		"language":           "Rust",
		"project_type":       "Web",
	})
	require.NoError(t, err)
	require.True(t, hints.HasCargoToml)
	require.True(t, hints.HasGoMod)
	require.True(t, hints.HasPythonFiles)
	require.True(t, hints.IsGitRepo)
	require.Equal(t, "Rust", hints.Language)
	require.Equal(t, "Web", hints.ProjectType)
	_, err = parseWorkspaceHints(map[string]any{"unknown": true})
	require.Error(t, err)

	require.Equal(t, []string{"a", "b"}, normalizeSlice([]string{" A ", "b", "A"}))
	require.True(t, contains([]string{"a", "b"}, "B"))
	require.True(t, containsNormalized([]string{"two-words"}, "two words"))
	require.Equal(t, []string{"a", "b"}, uniqueStrings([]string{"a", "b", "a", "", "b"}))
	require.Equal(t, []string{"a", "b"}, filterEmpty([]string{"a", "", "b"}))
	require.Equal(t, "tool family x,y k p", renderDiscoveryQuery(DiscoveryQuery{ToolName: "tool", Family: "family", Intent: []string{"x", "y"}, Keywords: []string{"k"}, RequiredParams: []string{"p"}}))
	require.Equal(t, "tool family", renderInstantiationQuery(InstantiationQuery{ToolName: "tool", Family: "family"}))
	require.Equal(t, map[string]any{"x": 1}, cloneMap(map[string]any{"x": 1}))
	require.Equal(t, "grep", commandFromEntry(richSearchEntry()))
	require.Equal(t, "workspace", workdirMode(richSearchEntry()))
	require.Equal(t, "fixed", workdirMode(catalog.ToolCatalogEntry{}))

	qd, err := (DiscoveryQuery{ToolName: "  CLI-Cargo  ", Aliases: []string{"GreP", "grep"}, Family: " Build ", Intent: []string{" Rust "}, Keywords: []string{"Cargo"}, RequiredParams: []string{" Path "}, PreferredOutput: " TEXT ", WorkspaceContext: WorkspaceHints{Language: "Rust", ProjectType: "Web"}, MaxResults: 0}).Normalize()
	require.NoError(t, err)
	require.Equal(t, "cli_cargo", qd.ToolName)
	require.Equal(t, []string{"grep"}, qd.Aliases)
	require.Equal(t, "build", qd.Family)
	require.Equal(t, []string{"rust"}, qd.Intent)
	require.Equal(t, []string{"cargo"}, qd.Keywords)
	require.Equal(t, []string{"path"}, qd.RequiredParams)
	require.Equal(t, "text", qd.PreferredOutput)
	require.Equal(t, 10, qd.MaxResults)

	qi, err := (InstantiationQuery{ToolName: "  CLI-Cargo  ", Aliases: []string{"GreP", "grep"}, Family: " Build ", Arguments: nil, WorkspaceContext: WorkspaceHints{Language: "Rust", ProjectType: "Web"}}).Normalize()
	require.NoError(t, err)
	require.Equal(t, "cli_cargo", qi.ToolName)
	require.Equal(t, []string{"grep"}, qi.Aliases)
	require.Equal(t, "build", qi.Family)
	require.Empty(t, qi.Arguments)
	require.Equal(t, "rust", qi.WorkspaceContext.Language)
	require.Equal(t, "web", qi.WorkspaceContext.ProjectType)
	require.Empty(t, qi.ArgumentString("missing"))
}

func TestQueryEngineSearchAndInstantiateCoverScoringHelpers(t *testing.T) {
	cat := testCatalog(t)
	engine := NewEngine(cat)

	entry := richSearchEntry()
	q := DiscoveryQuery{
		ToolName:        "cli_search",
		Aliases:         []string{"grep"},
		Family:          "text",
		Intent:          []string{"search"},
		Keywords:        []string{"structured"},
		RequiredParams:  []string{"path"},
		PreferredOutput: "text",
		WorkspaceContext: WorkspaceHints{
			HasCargoToml: true,
			Language:     "Rust",
			ProjectType:  "Web",
		},
		MaxResults: 5,
	}
	match := scoreEntry(entry, q)
	require.Greater(t, match.Score, 0)
	require.NotEmpty(t, match.Reasons)
	require.True(t, matchesKeyword(entry, "structured"))
	require.True(t, prefersOutput(entry, "text"))
	require.True(t, hasParameter(entry, "path"))
	require.False(t, hasParameter(entry, "missing"))
	cargoEntry, ok := cat.Lookup("cli_cargo")
	require.True(t, ok)
	reasons := []string{}
	require.Greater(t, workspaceBias(cargoEntry, WorkspaceHints{HasCargoToml: true, Language: "Rust"}, &reasons), 0)
	require.NotEmpty(t, reasons)

	result, err := engine.Search(q)
	require.NoError(t, err)
	require.NotEmpty(t, result.Matches)
	require.Equal(t, "cli_search", result.Matches[0].Entry.Name)
	require.Equal(t, 1, result.FamilySummary["text"])
	require.Contains(t, result.NormalizedQuery, "cli_search")

	inst, err := engine.Instantiate(InstantiationQuery{
		ToolName: "cli_search",
		Arguments: map[string]any{
			"path":               "src",
			"pattern":             "hello",
			"stdin":               "input",
			"working_directory":   "nested",
			"args":                []any{"--line-number"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "cli_search", inst.Match.Entry.Name)
	require.Equal(t, []string{"grep", "--line-number"}, inst.Request.Args)
	require.Equal(t, "input", inst.Request.Input)
	require.Equal(t, "workspace", inst.Preset.WorkdirMode)
	require.Equal(t, "workspace", workdirMode(inst.Match.Entry))
	require.Equal(t, "text", inst.Preset.Category)
	require.Equal(t, "src", inst.StructuredArgs["path"])
	require.Equal(t, "hello", inst.StructuredArgs["pattern"])
}

func TestQueryToolsExposeMetadataAndExecute(t *testing.T) {
	telemetry := &queryTelemetry{}
	tools := ToolsWithTelemetry(testCatalog(t), telemetry)
	require.Len(t, tools, 2)

	for _, tool := range tools {
		require.Equal(t, "shell-query", tool.Category())
		require.True(t, tool.IsAvailable(context.Background(), core.NewContext()))
		require.NotEmpty(t, tool.Description())
		require.NotEmpty(t, tool.Parameters())
		require.NotNil(t, tool.Permissions())
		require.NotEmpty(t, tool.Tags())
		switch tool.Name() {
		case discoveryToolName:
			require.Len(t, tool.Parameters(), 10)
			result, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
				"aliases":     []any{"grep"},
				"max_results": 3,
			})
			require.NoError(t, err)
			require.True(t, result.Success)
			require.Equal(t, "discovery", result.Metadata["query_type"])
			require.NotEmpty(t, result.Data["matches"])
		case instantiationToolName:
			result, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
				"tool_name": "cli_search",
				"arguments": map[string]any{
					"path":   "src",
					"pattern": "hello",
				},
			})
			require.NoError(t, err)
			require.True(t, result.Success)
			require.Equal(t, "instantiation", result.Metadata["query_type"])
			require.NotEmpty(t, result.Data["request"])
		}
	}
	require.NotEmpty(t, telemetry.events)
}

func TestParseQueryBranchesAndValidationFailures(t *testing.T) {
	discovery, err := ParseDiscoveryQuery(map[string]any{
		"tool_name": "  CLI-Cargo  ",
		"aliases":   []any{"GreP", "grep"},
		"family":    " Build ",
		"intent":    []string{" Rust "},
		"keywords":  "Cargo",
		"required_params": []any{
			"path",
		},
		"preferred_output": " TEXT ",
		"workspace_context": map[string]any{
			"has_cargo_toml":   "true",
			"has_go_mod":       true,
			"has_package_json": false,
			"has_python_files": true,
			"has_notebook_files": false,
			"is_git_repo":      true,
			"language":         "Rust",
			"project_type":     "Web",
		},
		"max_results":      "25",
		"allow_deprecated": "true",
	})
	require.NoError(t, err)
	require.Equal(t, "cli_cargo", discovery.ToolName)
	require.Equal(t, []string{"grep"}, discovery.Aliases)
	require.Equal(t, "build", discovery.Family)
	require.Equal(t, []string{"rust"}, discovery.Intent)
	require.Equal(t, []string{"cargo"}, discovery.Keywords)
	require.Equal(t, []string{"path"}, discovery.RequiredParams)
	require.Equal(t, "text", discovery.PreferredOutput)
	require.True(t, discovery.AllowDeprecated)
	require.Equal(t, 25, discovery.MaxResults)

	inst, err := ParseInstantiationQuery(map[string]any{
		"tool_name": "  CLI-Search  ",
		"aliases":   []any{"grep", "grep"},
		"family":    " Text ",
		"arguments": map[string]any{
			"path":    "src",
			"pattern": "hello",
			"stdin":   "input",
		},
		"workspace_context": map[string]any{
			"language":     "Rust",
			"project_type": "Web",
		},
		"allow_deprecated": false,
	})
	require.NoError(t, err)
	require.Equal(t, "cli_search", inst.ToolName)
	require.Equal(t, []string{"grep"}, inst.Aliases)
	require.Equal(t, "text", inst.Family)
	require.Equal(t, map[string]any{"path": "src", "pattern": "hello", "stdin": "input"}, inst.Arguments)
	require.Equal(t, "rust", inst.WorkspaceContext.Language)
	require.Equal(t, "web", inst.WorkspaceContext.ProjectType)
	require.False(t, inst.AllowDeprecated)
	require.Empty(t, inst.ArgumentString("missing"))

	_, err = ParseDiscoveryQuery(nil)
	require.Error(t, err)
	_, err = ParseInstantiationQuery(nil)
	require.Error(t, err)
	_, err = ParseDiscoveryQuery(map[string]any{"unknown": true})
	require.Error(t, err)
	_, err = ParseInstantiationQuery(map[string]any{"unknown": true})
	require.Error(t, err)

	require.Error(t, DiscoveryQuery{}.Validate())
	require.Error(t, InstantiationQuery{}.Validate())
	_, err = (DiscoveryQuery{MaxResults: -1, ToolName: "cli"}).Normalize()
	require.Error(t, err)
	_, err = (DiscoveryQuery{ToolName: "cli", MaxResults: 100}).Normalize()
	require.Error(t, err)
	instNorm, err := (InstantiationQuery{ToolName: "cli", Arguments: map[string]any{"bad": 1}}).Normalize()
	require.NoError(t, err)
	require.Equal(t, map[string]any{"bad": 1}, instNorm.Arguments)
}

func TestQueryEngineAndToolErrorPaths(t *testing.T) {
	require.Nil(t, discoveryMatchesToData(nil))
	require.Nil(t, cloneMap(nil))

	entry := richSearchEntry()
	require.Equal(t, "grep", commandFromEntry(entry))
	require.Equal(t, "", commandFromEntry(catalog.ToolCatalogEntry{}))
	require.Equal(t, "fixed", workdirMode(catalog.ToolCatalogEntry{}))
	require.Equal(t, map[string]interface{}{"name": "cli_search", "command": "grep", "default_args": []string{"--line-number"}, "description": entry.Description, "category": "text", "tags": entry.Tags, "timeout": time.Minute.String(), "allow_stdin": true, "workdir_mode": "workspace"}, presetToData(execute.CommandPreset{
		Name:        "cli_search",
		Command:     "grep",
		DefaultArgs: []string{"--line-number"},
		Description: entry.Description,
		Category:    "text",
		Tags:        entry.Tags,
		AllowStdin:  true,
		Timeout:     time.Minute,
		WorkdirMode: "workspace",
	}))
	require.Equal(t, map[string]interface{}{"workdir": "nested", "args": []string{"grep", "--line-number"}, "input": "stdin", "timeout": "1m0s"}, requestToData(sandbox.CommandRequest{
		Workdir: "nested",
		Args:    []string{"grep", "--line-number"},
		Input:   "stdin",
		Timeout: time.Minute,
	}))

	var nilDiscovery *discoveryTool
	res, err := nilDiscovery.Execute(context.Background(), core.NewContext(), nil)
	require.NoError(t, err)
	require.False(t, res.Success)
	require.Equal(t, "query engine missing", res.Error)

	var nilInstantiation *instantiationTool
	res, err = nilInstantiation.Execute(context.Background(), core.NewContext(), nil)
	require.NoError(t, err)
	require.False(t, res.Success)
	require.Equal(t, "query engine missing", res.Error)

	engine := NewEngine(nil)
	_, err = engine.Search(DiscoveryQuery{ToolName: "cli_search"})
	require.Error(t, err)
	_, err = engine.Instantiate(InstantiationQuery{ToolName: "cli_search"})
	require.Error(t, err)
}
