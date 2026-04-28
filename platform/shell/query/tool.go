package query

import (
	"context"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/platform/shell/catalog"
	"codeburg.org/lexbit/relurpify/platform/shell/execute"
	shelltelemetry "codeburg.org/lexbit/relurpify/platform/shell/telemetry"
)

const (
	discoveryToolName     = "shell_tool_discover"
	instantiationToolName = "shell_tool_instantiate"
)

// Tools returns the query tools backed by a catalog.
func Tools(cat *catalog.ToolCatalog) []contracts.Tool {
	return ToolsWithTelemetry(cat, nil)
}

// ToolsWithTelemetry returns query tools that can emit lightweight telemetry.
func ToolsWithTelemetry(cat *catalog.ToolCatalog, telemetry shelltelemetry.Sink) []contracts.Tool {
	engine := NewEngineWithTelemetry(cat, telemetry)
	return []contracts.Tool{
		&discoveryTool{engine: engine},
		&instantiationTool{engine: engine},
	}
}

type discoveryTool struct {
	engine *Engine
}

func (t *discoveryTool) Name() string { return discoveryToolName }
func (t *discoveryTool) Description() string {
	return "Searches the shell catalog using bounded discovery queries."
}
func (t *discoveryTool) Category() string { return "shell-query" }
func (t *discoveryTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{
		{Name: "tool_name", Type: "string", Description: "Canonical tool name to prioritize.", Required: false},
		{Name: "aliases", Type: "array", Description: "Tool aliases to match.", Required: false},
		{Name: "family", Type: "string", Description: "Family name to narrow the search.", Required: false},
		{Name: "intent", Type: "array", Description: "Intent keywords to match.", Required: false},
		{Name: "keywords", Type: "array", Description: "Free-form search keywords.", Required: false},
		{Name: "required_params", Type: "array", Description: "Parameter names that must be supported.", Required: false},
		{Name: "preferred_output", Type: "string", Description: "Preferred output style.", Required: false},
		{Name: "workspace_context", Type: "object", Description: "Workspace hints such as cargo, go, python, or git.", Required: false},
		{Name: "max_results", Type: "integer", Description: "Maximum number of matches to return, capped at 25.", Required: false, Default: defaultMaxResults},
		{Name: "allow_deprecated", Type: "boolean", Description: "Include deprecated tools in the search results.", Required: false},
	}
}

func (t *discoveryTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	if t == nil || t.engine == nil {
		return &contracts.ToolResult{Success: false, Error: "query engine missing"}, nil
	}
	q, err := ParseDiscoveryQuery(args)
	if err != nil {
		return &contracts.ToolResult{Success: false, Error: err.Error()}, nil
	}
	result, err := t.engine.Search(q)
	if err != nil {
		return &contracts.ToolResult{Success: false, Error: err.Error()}, nil
	}
	return &contracts.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"normalized_query": result.NormalizedQuery,
			"family_summary":   result.FamilySummary,
			"matches":          discoveryMatchesToData(result.Matches),
		},
		Metadata: map[string]interface{}{
			"query_type": "discovery",
			"count":      len(result.Matches),
		},
	}, nil
}

func (t *discoveryTool) IsAvailable(context.Context) bool {
	return t != nil && t.engine != nil
}
func (t *discoveryTool) Permissions() contracts.ToolPermissions { return contracts.ToolPermissions{} }
func (t *discoveryTool) Tags() []string                         { return []string{contracts.TagReadOnly, "search"} }

type instantiationTool struct {
	engine *Engine
}

func (t *instantiationTool) Name() string { return instantiationToolName }
func (t *instantiationTool) Description() string {
	return "Resolves a shell catalog entry and materializes a validated request."
}
func (t *instantiationTool) Category() string { return "shell-query" }
func (t *instantiationTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{
		{Name: "tool_name", Type: "string", Description: "Canonical tool name to resolve.", Required: false},
		{Name: "aliases", Type: "array", Description: "Tool aliases to resolve.", Required: false},
		{Name: "family", Type: "string", Description: "Family name to resolve if unambiguous.", Required: false},
		{Name: "arguments", Type: "object", Description: "Structured tool arguments to validate and materialize.", Required: false},
		{Name: "workspace_context", Type: "object", Description: "Workspace hints such as cargo, go, python, or git.", Required: false},
		{Name: "allow_deprecated", Type: "boolean", Description: "Allow deprecated tools to be resolved.", Required: false},
	}
}

func (t *instantiationTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	if t == nil || t.engine == nil {
		return &contracts.ToolResult{Success: false, Error: "query engine missing"}, nil
	}
	q, err := ParseInstantiationQuery(args)
	if err != nil {
		return &contracts.ToolResult{Success: false, Error: err.Error()}, nil
	}
	result, err := t.engine.Instantiate(q)
	if err != nil {
		return &contracts.ToolResult{Success: false, Error: err.Error()}, nil
	}
	return &contracts.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"normalized_query": result.NormalizedQuery,
			"tool":             discoveryMatchToData(result.Match),
			"preset":           presetToData(result.Preset),
			"request":          requestToData(result.Request),
			"structured_args":  cloneMap(result.StructuredArgs),
		},
		Metadata: map[string]interface{}{
			"query_type": "instantiation",
			"tool_name":  result.Match.Entry.Name,
		},
	}, nil
}

func (t *instantiationTool) IsAvailable(context.Context) bool {
	return t != nil && t.engine != nil
}
func (t *instantiationTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{}
}
func (t *instantiationTool) Tags() []string { return []string{contracts.TagReadOnly, "search"} }

func discoveryMatchesToData(matches []DiscoveryMatch) []map[string]interface{} {
	if len(matches) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(matches))
	for _, match := range matches {
		out = append(out, discoveryMatchToData(match))
	}
	return out
}

func discoveryMatchToData(match DiscoveryMatch) map[string]interface{} {
	return map[string]interface{}{
		"name":              match.Entry.Name,
		"aliases":           append([]string(nil), match.Entry.Aliases...),
		"family":            match.Entry.Family,
		"intent":            append([]string(nil), match.Entry.Intent...),
		"description":       match.Entry.Description,
		"long_description":  match.Entry.LongDescription,
		"tags":              append([]string(nil), match.Entry.Tags...),
		"deprecated":        match.Entry.Deprecated,
		"replacement":       match.Entry.Replacement,
		"score":             match.Score,
		"reasons":           append([]string(nil), match.Reasons...),
		"parameter_summary": append([]string(nil), match.ParameterSummary...),
		"examples":          append([]catalog.ToolExample(nil), match.Examples...),
	}
}

func presetToData(p execute.CommandPreset) map[string]interface{} {
	return map[string]interface{}{
		"name":         p.Name,
		"command":      p.Command,
		"default_args": append([]string(nil), p.DefaultArgs...),
		"description":  p.Description,
		"category":     p.Category,
		"tags":         append([]string(nil), p.Tags...),
		"timeout":      p.Timeout.String(),
		"allow_stdin":  p.AllowStdin,
		"workdir_mode": p.WorkdirMode,
	}
}

func requestToData(req contracts.CommandRequest) map[string]interface{} {
	return map[string]interface{}{
		"workdir": req.Workdir,
		"args":    append([]string(nil), req.Args...),
		"input":   req.Input,
		"timeout": req.Timeout.String(),
	}
}
