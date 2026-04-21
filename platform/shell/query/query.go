package query

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/platform/shell/catalog"
	"codeburg.org/lexbit/relurpify/platform/shell/execute"
	shelltelemetry "codeburg.org/lexbit/relurpify/platform/shell/telemetry"
)

const defaultMaxResults = 10
const maxDiscoveryResults = 25

// WorkspaceHints bias tool ranking without blocking results.
type WorkspaceHints struct {
	HasCargoToml     bool
	HasGoMod         bool
	HasPackageJSON   bool
	HasPythonFiles   bool
	HasNotebookFiles bool
	IsGitRepo        bool
	Language         string
	ProjectType      string
}

// DiscoveryQuery searches for tools by intent and context.
type DiscoveryQuery struct {
	ToolName         string
	Aliases          []string
	Family           string
	Intent           []string
	Keywords         []string
	RequiredParams   []string
	PreferredOutput  string
	WorkspaceContext WorkspaceHints
	MaxResults       int
	AllowDeprecated  bool
}

// InstantiationQuery resolves a tool and materializes a request envelope.
type InstantiationQuery struct {
	ToolName         string
	Aliases          []string
	Family           string
	Arguments        map[string]any
	WorkspaceContext WorkspaceHints
	AllowDeprecated  bool
}

// DiscoveryMatch represents a ranked candidate.
type DiscoveryMatch struct {
	Entry            catalog.ToolCatalogEntry
	Score            int
	Reasons          []string
	ParameterSummary []string
	Examples         []catalog.ToolExample
}

// DiscoveryResult returns ranked discovery output.
type DiscoveryResult struct {
	OriginalQuery   DiscoveryQuery
	NormalizedQuery string
	Matches         []DiscoveryMatch
	FamilySummary   map[string]int
}

// InstantiationResult carries the chosen tool and a framework-facing request.
type InstantiationResult struct {
	OriginalQuery   InstantiationQuery
	NormalizedQuery string
	Match           DiscoveryMatch
	Preset          execute.CommandPreset
	Request         sandbox.CommandRequest
	StructuredArgs  map[string]any
}

// Engine evaluates discovery and instantiation queries against a catalog.
type Engine struct {
	catalog   *catalog.ToolCatalog
	telemetry shelltelemetry.Sink
}

// NewEngine creates a query engine for a catalog.
func NewEngine(cat *catalog.ToolCatalog) *Engine {
	return NewEngineWithTelemetry(cat, nil)
}

// NewEngineWithTelemetry creates a query engine that can emit lightweight telemetry.
func NewEngineWithTelemetry(cat *catalog.ToolCatalog, telemetry shelltelemetry.Sink) *Engine {
	return &Engine{catalog: cat, telemetry: telemetry}
}

// ParseDiscoveryQuery validates and normalizes a raw discovery payload.
func ParseDiscoveryQuery(raw map[string]any) (DiscoveryQuery, error) {
	q := DiscoveryQuery{MaxResults: defaultMaxResults}
	if raw == nil {
		return q, fmt.Errorf("discovery query missing")
	}
	for key, value := range raw {
		switch catalog.NormalizeName(key) {
		case "tool_name":
			s, err := asString(value)
			if err != nil {
				return q, fmt.Errorf("tool_name: %w", err)
			}
			q.ToolName = s
		case "aliases":
			list, err := asStringSlice(value)
			if err != nil {
				return q, fmt.Errorf("aliases: %w", err)
			}
			q.Aliases = list
		case "family":
			s, err := asString(value)
			if err != nil {
				return q, fmt.Errorf("family: %w", err)
			}
			q.Family = s
		case "intent":
			list, err := asStringSlice(value)
			if err != nil {
				return q, fmt.Errorf("intent: %w", err)
			}
			q.Intent = list
		case "keywords":
			list, err := asStringSlice(value)
			if err != nil {
				return q, fmt.Errorf("keywords: %w", err)
			}
			q.Keywords = list
		case "required_params":
			list, err := asStringSlice(value)
			if err != nil {
				return q, fmt.Errorf("required_params: %w", err)
			}
			q.RequiredParams = list
		case "preferred_output":
			s, err := asString(value)
			if err != nil {
				return q, fmt.Errorf("preferred_output: %w", err)
			}
			q.PreferredOutput = s
		case "workspace_context":
			hints, err := parseWorkspaceHints(value)
			if err != nil {
				return q, fmt.Errorf("workspace_context: %w", err)
			}
			q.WorkspaceContext = hints
		case "max_results":
			n, err := asInt(value)
			if err != nil {
				return q, fmt.Errorf("max_results: %w", err)
			}
			q.MaxResults = n
		case "allow_deprecated":
			b, err := asBool(value)
			if err != nil {
				return q, fmt.Errorf("allow_deprecated: %w", err)
			}
			q.AllowDeprecated = b
		default:
			return q, fmt.Errorf("unknown discovery field %q", key)
		}
	}
	return q.Normalize()
}

// ParseInstantiationQuery validates and normalizes a raw instantiation payload.
func ParseInstantiationQuery(raw map[string]any) (InstantiationQuery, error) {
	q := InstantiationQuery{}
	if raw == nil {
		return q, fmt.Errorf("instantiation query missing")
	}
	for key, value := range raw {
		switch catalog.NormalizeName(key) {
		case "tool_name":
			s, err := asString(value)
			if err != nil {
				return q, fmt.Errorf("tool_name: %w", err)
			}
			q.ToolName = s
		case "aliases":
			list, err := asStringSlice(value)
			if err != nil {
				return q, fmt.Errorf("aliases: %w", err)
			}
			q.Aliases = list
		case "family":
			s, err := asString(value)
			if err != nil {
				return q, fmt.Errorf("family: %w", err)
			}
			q.Family = s
		case "arguments":
			args, err := asStringMap(value)
			if err != nil {
				return q, fmt.Errorf("arguments: %w", err)
			}
			q.Arguments = args
		case "workspace_context":
			hints, err := parseWorkspaceHints(value)
			if err != nil {
				return q, fmt.Errorf("workspace_context: %w", err)
			}
			q.WorkspaceContext = hints
		case "allow_deprecated":
			b, err := asBool(value)
			if err != nil {
				return q, fmt.Errorf("allow_deprecated: %w", err)
			}
			q.AllowDeprecated = b
		default:
			return q, fmt.Errorf("unknown instantiation field %q", key)
		}
	}
	return q.Normalize()
}

// Normalize canonicalizes the discovery query.
func (q DiscoveryQuery) Normalize() (DiscoveryQuery, error) {
	q.ToolName = catalog.NormalizeName(q.ToolName)
	q.Aliases = normalizeSlice(q.Aliases)
	q.Family = catalog.NormalizeName(q.Family)
	q.Intent = normalizeSlice(q.Intent)
	q.Keywords = normalizeSlice(q.Keywords)
	q.RequiredParams = normalizeSlice(q.RequiredParams)
	q.PreferredOutput = strings.TrimSpace(strings.ToLower(q.PreferredOutput))
	q.WorkspaceContext.Language = strings.TrimSpace(strings.ToLower(q.WorkspaceContext.Language))
	q.WorkspaceContext.ProjectType = strings.TrimSpace(strings.ToLower(q.WorkspaceContext.ProjectType))
	if q.MaxResults < 0 {
		return q, fmt.Errorf("max_results must be non-negative")
	}
	if q.MaxResults > maxDiscoveryResults {
		return q, fmt.Errorf("max_results must be at most %d", maxDiscoveryResults)
	}
	if q.MaxResults == 0 {
		q.MaxResults = defaultMaxResults
	}
	if err := q.Validate(); err != nil {
		return q, err
	}
	return q, nil
}

// Normalize canonicalizes the instantiation query.
func (q InstantiationQuery) Normalize() (InstantiationQuery, error) {
	q.ToolName = catalog.NormalizeName(q.ToolName)
	q.Aliases = normalizeSlice(q.Aliases)
	q.Family = catalog.NormalizeName(q.Family)
	if q.Arguments == nil {
		q.Arguments = map[string]any{}
	}
	q.WorkspaceContext.Language = strings.TrimSpace(strings.ToLower(q.WorkspaceContext.Language))
	q.WorkspaceContext.ProjectType = strings.TrimSpace(strings.ToLower(q.WorkspaceContext.ProjectType))
	if err := q.Validate(); err != nil {
		return q, err
	}
	return q, nil
}

// ArgumentString returns a string argument from the structured payload.
func (q InstantiationQuery) ArgumentString(name string) string {
	if q.Arguments == nil {
		return ""
	}
	value, ok := q.Arguments[name]
	if !ok || value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

// Validate ensures the discovery query is usable.
func (q DiscoveryQuery) Validate() error {
	if q.ToolName == "" && len(q.Aliases) == 0 && q.Family == "" && len(q.Intent) == 0 && len(q.Keywords) == 0 && !q.WorkspaceContext.HasCargoToml && !q.WorkspaceContext.HasGoMod && !q.WorkspaceContext.HasPackageJSON && !q.WorkspaceContext.HasPythonFiles && !q.WorkspaceContext.HasNotebookFiles && !q.WorkspaceContext.IsGitRepo {
		return fmt.Errorf("discovery query must include at least one search signal")
	}
	if q.MaxResults < 0 {
		return fmt.Errorf("max_results must be non-negative")
	}
	if q.MaxResults > maxDiscoveryResults {
		return fmt.Errorf("max_results must be at most %d", maxDiscoveryResults)
	}
	return nil
}

// Validate ensures the instantiation query has enough information to resolve a tool.
func (q InstantiationQuery) Validate() error {
	if q.ToolName == "" && len(q.Aliases) == 0 && q.Family == "" {
		return fmt.Errorf("instantiation query must name a tool, alias, or family")
	}
	return nil
}

// Search returns ranked discovery matches.
func (e *Engine) Search(q DiscoveryQuery) (*DiscoveryResult, error) {
	if e == nil || e.catalog == nil {
		return nil, fmt.Errorf("catalog missing")
	}
	q, err := q.Normalize()
	if err != nil {
		e.emitTelemetry("tool_result", "shell discovery query validation failed", map[string]any{
			"query_type": "discovery",
			"status":     "failed",
			"error":      err.Error(),
		})
		return nil, err
	}
	e.emitTelemetry("tool_call", "shell discovery query started", map[string]any{
		"query_type":       "discovery",
		"tool_name":        q.ToolName,
		"aliases":          append([]string(nil), q.Aliases...),
		"family":           q.Family,
		"max_results":      q.MaxResults,
		"allow_deprecated": q.AllowDeprecated,
	})
	matches := make([]DiscoveryMatch, 0)
	for _, entry := range e.catalog.List() {
		if entry.Deprecated && !q.AllowDeprecated {
			continue
		}
		match := scoreEntry(entry, q)
		if match.Score <= 0 {
			continue
		}
		matches = append(matches, match)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Entry.Name < matches[j].Entry.Name
	})
	if len(matches) > q.MaxResults {
		matches = matches[:q.MaxResults]
	}
	summary := make(map[string]int)
	for _, match := range matches {
		summary[match.Entry.Family]++
	}
	e.emitTelemetry("tool_result", "shell discovery query completed", map[string]any{
		"query_type": "discovery",
		"status":     "success",
		"count":      len(matches),
		"families":   summary,
	})
	return &DiscoveryResult{
		OriginalQuery:   q,
		NormalizedQuery: renderDiscoveryQuery(q),
		Matches:         matches,
		FamilySummary:   summary,
	}, nil
}

// Instantiate resolves a tool and builds a request envelope.
func (e *Engine) Instantiate(q InstantiationQuery) (*InstantiationResult, error) {
	if e == nil || e.catalog == nil {
		return nil, fmt.Errorf("catalog missing")
	}
	q, err := q.Normalize()
	if err != nil {
		e.emitTelemetry("tool_result", "shell instantiation query validation failed", map[string]any{
			"query_type": "instantiation",
			"status":     "failed",
			"error":      err.Error(),
		})
		return nil, err
	}
	e.emitTelemetry("tool_call", "shell instantiation query started", map[string]any{
		"query_type": "instantiation",
		"tool_name":  q.ToolName,
		"aliases":    append([]string(nil), q.Aliases...),
		"family":     q.Family,
	})
	entry, err := e.resolveEntry(q)
	if err != nil {
		e.emitTelemetry("tool_result", "shell instantiation query failed", map[string]any{
			"query_type": "instantiation",
			"status":     "failed",
			"error":      err.Error(),
		})
		return nil, err
	}
	if entry.Name == "" {
		e.emitTelemetry("tool_result", "shell instantiation query failed", map[string]any{
			"query_type": "instantiation",
			"status":     "failed",
			"error":      "tool not found",
		})
		return nil, fmt.Errorf("tool not found")
	}
	if entry.Deprecated && !q.AllowDeprecated {
		e.emitTelemetry("tool_result", "shell instantiation query rejected deprecated tool", map[string]any{
			"query_type": "instantiation",
			"status":     "failed",
			"tool_name":  entry.Name,
			"deprecated": true,
		})
		return nil, fmt.Errorf("tool %q is deprecated", entry.Name)
	}
	if err := validateInstantiationArgs(entry, q.Arguments); err != nil {
		e.emitTelemetry("tool_result", "shell instantiation query validation failed", map[string]any{
			"query_type": "instantiation",
			"status":     "failed",
			"tool_name":  entry.Name,
			"error":      err.Error(),
		})
		return nil, err
	}
	args := buildCLIArgs(entry, q.Arguments)
	wd := ""
	if entry.Preset.SupportsWorkdir {
		wd = q.ArgumentString("working_directory")
	}
	stdin := ""
	if entry.Preset.AllowStdin {
		stdin = q.ArgumentString("stdin")
	}
	result := DiscoveryMatch{
		Entry:            entry,
		Score:            1,
		Reasons:          []string{"resolved"},
		ParameterSummary: parameterSummary(entry),
		Examples:         append([]catalog.ToolExample(nil), entry.Examples...),
	}
	e.emitTelemetry("tool_result", "shell instantiation query completed", map[string]any{
		"query_type": "instantiation",
		"status":     "success",
		"tool_name":  entry.Name,
		"deprecated": entry.Deprecated,
	})
	return &InstantiationResult{
		OriginalQuery:   q,
		NormalizedQuery: renderInstantiationQuery(q),
		Match:           result,
		Preset: execute.CommandPreset{
			Name:        entry.Name,
			Command:     commandFromEntry(entry),
			DefaultArgs: append([]string(nil), entry.Preset.DefaultArgs...),
			Description: entry.Description,
			Category:    entry.Family,
			Tags:        append([]string(nil), entry.Tags...),
			Timeout:     60 * time.Second,
			AllowStdin:  entry.Preset.AllowStdin,
			WorkdirMode: workdirMode(entry),
		},
		Request: sandbox.CommandRequest{
			Args:    args,
			Workdir: wd,
			Input:   stdin,
		},
		StructuredArgs: cloneMap(q.Arguments),
	}, nil
}

func (e *Engine) resolveEntry(q InstantiationQuery) (catalog.ToolCatalogEntry, error) {
	if q.ToolName != "" {
		if entry, ok := e.catalog.Lookup(q.ToolName); ok {
			if catalog.NormalizeName(q.ToolName) != entry.Name {
				e.emitTelemetry("state_change", "shell alias resolved", map[string]any{
					"query_type": "instantiation",
					"alias":      q.ToolName,
					"tool_name":  entry.Name,
				})
			}
			return entry, nil
		}
	}
	for _, alias := range q.Aliases {
		if entry, ok := e.catalog.Lookup(alias); ok {
			e.emitTelemetry("state_change", "shell alias resolved", map[string]any{
				"query_type": "instantiation",
				"alias":      alias,
				"tool_name":  entry.Name,
			})
			return entry, nil
		}
	}
	if q.Family != "" {
		var candidates []catalog.ToolCatalogEntry
		for _, entry := range e.catalog.List() {
			if entry.Family == q.Family {
				candidates = append(candidates, entry)
			}
		}
		if len(candidates) == 1 {
			return candidates[0], nil
		}
		if len(candidates) > 1 {
			return catalog.ToolCatalogEntry{}, fmt.Errorf("family %q is ambiguous; specify a tool_name or alias", q.Family)
		}
	}
	return catalog.ToolCatalogEntry{}, nil
}

func scoreEntry(entry catalog.ToolCatalogEntry, q DiscoveryQuery) DiscoveryMatch {
	score := 0
	var reasons []string
	if q.ToolName != "" && entry.Name == q.ToolName {
		score += 100
		reasons = append(reasons, "name")
	}
	for _, alias := range q.Aliases {
		if containsNormalized(entry.Aliases, alias) {
			score += 90
			reasons = append(reasons, "alias:"+alias)
		}
	}
	if q.Family != "" && entry.Family == q.Family {
		score += 30
		reasons = append(reasons, "family")
	}
	for _, intent := range q.Intent {
		if contains(entry.Intent, intent) {
			score += 40
			reasons = append(reasons, "intent:"+intent)
		}
	}
	for _, keyword := range q.Keywords {
		if matchesKeyword(entry, keyword) {
			score += 10
			reasons = append(reasons, "keyword:"+keyword)
		}
	}
	for _, param := range q.RequiredParams {
		if hasParameter(entry, param) {
			score += 20
			reasons = append(reasons, "param:"+param)
		}
	}
	if q.PreferredOutput != "" && prefersOutput(entry, q.PreferredOutput) {
		score += 15
		reasons = append(reasons, "preferred_output")
	}
	score += workspaceBias(entry, q.WorkspaceContext, &reasons)
	if entry.Deprecated {
		score -= 5
		reasons = append(reasons, "deprecated")
	}
	return DiscoveryMatch{
		Entry:            entry,
		Score:            score,
		Reasons:          uniqueStrings(reasons),
		ParameterSummary: parameterSummary(entry),
		Examples:         append([]catalog.ToolExample(nil), entry.Examples...),
	}
}

func workspaceBias(entry catalog.ToolCatalogEntry, hints WorkspaceHints, reasons *[]string) int {
	score := 0
	if hints.HasCargoToml && (entry.Name == "cli_cargo" || contains(entry.Intent, "rust")) {
		score += 25
		*reasons = append(*reasons, "workspace:cargo")
	}
	if hints.HasGoMod && (entry.Name == "cli_go" || contains(entry.Intent, "go")) {
		score += 25
		*reasons = append(*reasons, "workspace:go")
	}
	if hints.HasPackageJSON && (entry.Name == "cli_node" || entry.Name == "cli_npm" || contains(entry.Intent, "node")) {
		score += 25
		*reasons = append(*reasons, "workspace:node")
	}
	if hints.HasPythonFiles && (entry.Name == "cli_python" || contains(entry.Intent, "python")) {
		score += 25
		*reasons = append(*reasons, "workspace:python")
	}
	if hints.IsGitRepo && (entry.Name == "cli_git" || contains(entry.Intent, "repository")) {
		score += 15
		*reasons = append(*reasons, "workspace:git")
	}
	if strings.Contains(hints.Language, "rust") && contains(entry.Intent, "rust") {
		score += 10
	}
	if strings.Contains(hints.ProjectType, "web") && (contains(entry.Intent, "http") || contains(entry.Intent, "node")) {
		score += 5
	}
	return score
}

func matchesKeyword(entry catalog.ToolCatalogEntry, keyword string) bool {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return false
	}
	if strings.Contains(strings.ToLower(entry.Name), keyword) || strings.Contains(strings.ToLower(entry.Description), keyword) || strings.Contains(strings.ToLower(entry.LongDescription), keyword) || strings.Contains(strings.ToLower(entry.Family), keyword) {
		return true
	}
	for _, alias := range entry.Aliases {
		if strings.Contains(alias, keyword) {
			return true
		}
	}
	for _, intent := range entry.Intent {
		if strings.Contains(intent, keyword) {
			return true
		}
	}
	for _, tag := range entry.Tags {
		if strings.Contains(tag, keyword) {
			return true
		}
	}
	for _, ex := range entry.Examples {
		if strings.Contains(strings.ToLower(ex.Query), keyword) || strings.Contains(strings.ToLower(ex.Output), keyword) {
			return true
		}
		for _, value := range ex.Input {
			if strings.Contains(strings.ToLower(fmt.Sprint(value)), keyword) {
				return true
			}
		}
	}
	return false
}

func prefersOutput(entry catalog.ToolCatalogEntry, preferred string) bool {
	preferred = strings.ToLower(strings.TrimSpace(preferred))
	if preferred == "" {
		return false
	}
	if strings.Contains(strings.ToLower(entry.Preset.ResultStyle), preferred) || strings.Contains(strings.ToLower(entry.Description), preferred) {
		return true
	}
	for _, ex := range entry.Examples {
		if strings.Contains(strings.ToLower(ex.Output), preferred) {
			return true
		}
	}
	return false
}

func hasParameter(entry catalog.ToolCatalogEntry, name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	if _, ok := entry.ParameterSchema.Properties[name]; ok {
		return true
	}
	return contains(entry.ParameterSchema.Required, name)
}

func buildCLIArgs(entry catalog.ToolCatalogEntry, args map[string]any) []string {
	cmd := append([]string{}, entry.Preset.CommandTemplate...)
	cmd = append(cmd, entry.Preset.DefaultArgs...)
	if raw, ok := args["args"]; ok {
		if extra, err := asStringSlice(raw); err == nil {
			cmd = append(cmd, extra...)
		}
	}
	return cmd
}

func validateInstantiationArgs(entry catalog.ToolCatalogEntry, args map[string]any) error {
	if len(entry.ParameterSchema.Required) == 0 && len(entry.ParameterSchema.Properties) == 0 {
		return nil
	}
	for _, name := range entry.ParameterSchema.Required {
		if _, ok := args[name]; !ok {
			return fmt.Errorf("missing required parameter %q", name)
		}
	}
	for name := range args {
		if name == "args" || name == "working_directory" || name == "stdin" {
			continue
		}
		if _, ok := entry.ParameterSchema.Properties[name]; !ok {
			return fmt.Errorf("unknown parameter %q", name)
		}
	}
	return nil
}

func parameterSummary(entry catalog.ToolCatalogEntry) []string {
	var out []string
	if len(entry.ParameterSchema.Required) > 0 || len(entry.ParameterSchema.Properties) > 0 {
		out = append(out, entry.ParameterSchema.Required...)
		for name := range entry.ParameterSchema.Properties {
			if !contains(out, name) {
				out = append(out, name)
			}
		}
	}
	out = append(out, "args")
	if entry.Preset.SupportsWorkdir {
		out = append(out, "working_directory")
	}
	if entry.Preset.AllowStdin {
		out = append(out, "stdin")
	}
	return uniqueStrings(out)
}

func workdirMode(entry catalog.ToolCatalogEntry) string {
	if entry.Preset.SupportsWorkdir {
		return "workspace"
	}
	return "fixed"
}

func commandFromEntry(entry catalog.ToolCatalogEntry) string {
	if len(entry.Preset.CommandTemplate) > 0 {
		return entry.Preset.CommandTemplate[0]
	}
	return ""
}

func renderDiscoveryQuery(q DiscoveryQuery) string {
	parts := []string{q.ToolName, q.Family, strings.Join(q.Intent, ","), strings.Join(q.Keywords, ","), strings.Join(q.RequiredParams, ",")}
	return strings.Join(filterEmpty(parts), " ")
}

func renderInstantiationQuery(q InstantiationQuery) string {
	parts := []string{q.ToolName, q.Family}
	return strings.Join(filterEmpty(parts), " ")
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func filterEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func normalizeSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := catalog.NormalizeName(value)
		if normalized != "" {
			out = append(out, normalized)
		}
	}
	return uniqueStrings(out)
}

func contains(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, value := range values {
		if strings.ToLower(value) == target {
			return true
		}
	}
	return false
}

func containsNormalized(values []string, target string) bool {
	target = catalog.NormalizeName(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if catalog.NormalizeName(value) == target {
			return true
		}
	}
	return false
}

func (e *Engine) emitTelemetry(eventType string, message string, metadata map[string]any) {
	if e == nil || e.telemetry == nil {
		return
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	e.telemetry.Emit(shelltelemetry.Event{
		Type:      eventType,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	})
}

func asString(value any) (string, error) {
	if value == nil {
		return "", fmt.Errorf("string value required")
	}
	if s, ok := value.(string); ok {
		return s, nil
	}
	return "", fmt.Errorf("expected string, got %T", value)
}

func asStringSlice(value any) ([]string, error) {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...), nil
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			s, err := asString(item)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, nil
	case string:
		if typed == "" {
			return nil, nil
		}
		return []string{typed}, nil
	default:
		return nil, fmt.Errorf("expected string slice, got %T", value)
	}
}

func asStringMap(value any) (map[string]any, error) {
	if value == nil {
		return map[string]any{}, nil
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected object, got %T", value)
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out, nil
}

func asBool(value any) (bool, error) {
	if value == nil {
		return false, fmt.Errorf("bool value required")
	}
	switch typed := value.(type) {
	case bool:
		return typed, nil
	case string:
		b, err := strconv.ParseBool(typed)
		if err != nil {
			return false, err
		}
		return b, nil
	default:
		return false, fmt.Errorf("expected bool, got %T", value)
	}
}

func asInt(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int32:
		return int(typed), nil
	case int64:
		return int(typed), nil
	case float64:
		return int(typed), nil
	case string:
		n, err := strconv.Atoi(typed)
		if err != nil {
			return 0, err
		}
		return n, nil
	default:
		return 0, fmt.Errorf("expected int, got %T", value)
	}
}

func parseWorkspaceHints(value any) (WorkspaceHints, error) {
	if value == nil {
		return WorkspaceHints{}, nil
	}
	m, ok := value.(map[string]any)
	if !ok {
		return WorkspaceHints{}, fmt.Errorf("expected object, got %T", value)
	}
	var hints WorkspaceHints
	for key, raw := range m {
		switch catalog.NormalizeName(key) {
		case "has_cargo_toml":
			b, err := asBool(raw)
			if err != nil {
				return hints, err
			}
			hints.HasCargoToml = b
		case "has_go_mod":
			b, err := asBool(raw)
			if err != nil {
				return hints, err
			}
			hints.HasGoMod = b
		case "has_package_json":
			b, err := asBool(raw)
			if err != nil {
				return hints, err
			}
			hints.HasPackageJSON = b
		case "has_python_files":
			b, err := asBool(raw)
			if err != nil {
				return hints, err
			}
			hints.HasPythonFiles = b
		case "has_notebook_files":
			b, err := asBool(raw)
			if err != nil {
				return hints, err
			}
			hints.HasNotebookFiles = b
		case "is_git_repo":
			b, err := asBool(raw)
			if err != nil {
				return hints, err
			}
			hints.IsGitRepo = b
		case "language":
			s, err := asString(raw)
			if err != nil {
				return hints, err
			}
			hints.Language = s
		case "project_type":
			s, err := asString(raw)
			if err != nil {
				return hints, err
			}
			hints.ProjectType = s
		default:
			return hints, fmt.Errorf("unknown workspace_context field %q", key)
		}
	}
	return hints, nil
}

func cloneMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
