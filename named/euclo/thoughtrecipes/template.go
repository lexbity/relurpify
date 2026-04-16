package thoughtrecipes

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
)

// TemplateContext is the bounded namespace available inside ${...} substitutions.
type TemplateContext struct {
	Task       TemplateTaskView
	Context    map[string]string // alias name → rendered string value
	Enrichment TemplateEnrichmentView
}

// TemplateTaskView provides task-level variables for template substitution.
type TemplateTaskView struct {
	Instruction string
	Type        string // "analysis" | "code_modification" | "planning" | "review"
	Workspace   string
}

// TemplateEnrichmentView provides enrichment source summaries for template substitution.
type TemplateEnrichmentView struct {
	AST         string // empty if ast not enabled
	Archaeology string // empty if archaeology not enabled
	BKC         string // empty if bkc not enabled
}

// EnrichmentBundle holds the loaded enrichment data for template rendering.
type EnrichmentBundle struct {
	AST         string
	Archaeology string
	BKC         string
}

// varPattern matches ${variable.name} patterns in templates.
// It captures the variable path (e.g., "task.instruction" or "context.explore_findings")
var varPattern = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_.]*)\}`)

// RenderPrompt performs ${var} substitution on tmpl using ctx.
// Unresolved variables render as empty string; each unresolved variable
// appends an entry to the returned warnings slice.
func RenderPrompt(tmpl string, ctx TemplateContext) (rendered string, warnings []string) {
	if tmpl == "" {
		return "", nil
	}

	warnings = make([]string, 0)

	result := varPattern.ReplaceAllStringFunc(tmpl, func(match string) string {
		// Extract the variable path from ${...}
		path := match[2 : len(match)-1] // remove ${ and }

		value, ok := resolveVariable(path, ctx)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("unresolved template variable: %s", path))
			return ""
		}
		return value
	})

	return result, warnings
}

// resolveVariable resolves a dotted variable path against the template context.
// Returns (value, true) if resolved, ("", false) if not found.
func resolveVariable(path string, ctx TemplateContext) (string, bool) {
	parts := strings.SplitN(path, ".", 2)
	if len(parts) == 0 {
		return "", false
	}

	namespace := parts[0]
	var subpath string
	if len(parts) > 1 {
		subpath = parts[1]
	}

	switch namespace {
	case "task":
		return resolveTaskVariable(subpath, ctx.Task)
	case "context":
		return resolveContextVariable(subpath, ctx.Context)
	case "enrichment":
		return resolveEnrichmentVariable(subpath, ctx.Enrichment)
	default:
		return "", false
	}
}

// resolveTaskVariable resolves task.* variables.
func resolveTaskVariable(subpath string, task TemplateTaskView) (string, bool) {
	switch subpath {
	case "instruction":
		return task.Instruction, true
	case "type":
		return task.Type, true
	case "workspace":
		return task.Workspace, true
	default:
		return "", false
	}
}

// resolveContextVariable resolves context.* variables (alias lookups).
func resolveContextVariable(aliasName string, contextMap map[string]string) (string, bool) {
	if aliasName == "" {
		return "", false
	}
	value, ok := contextMap[aliasName]
	return value, ok
}

// resolveEnrichmentVariable resolves enrichment.* variables.
func resolveEnrichmentVariable(subpath string, enrichment TemplateEnrichmentView) (string, bool) {
	switch subpath {
	case "ast":
		return enrichment.AST, true // may be empty string if not enabled, but that's valid
	case "archaeology":
		return enrichment.Archaeology, true
	case "bkc":
		return enrichment.BKC, true
	default:
		return "", false
	}
}

// BuildTemplateContext constructs the template context for a step from the
// current recipe execution state and the enrichment sources loaded for this step.
func BuildTemplateContext(task *core.Task, stepState *core.Context, enrichment EnrichmentBundle, resolver *AliasResolver) TemplateContext {
	ctx := TemplateContext{
		Task:    buildTaskView(task),
		Context: buildContextMap(stepState, resolver),
		Enrichment: TemplateEnrichmentView{
			AST:         enrichment.AST,
			Archaeology: enrichment.Archaeology,
			BKC:         enrichment.BKC,
		},
	}
	return ctx
}

// buildTaskView constructs the TemplateTaskView from a core.Task.
func buildTaskView(task *core.Task) TemplateTaskView {
	if task == nil {
		return TemplateTaskView{}
	}

	view := TemplateTaskView{
		Instruction: task.Instruction,
		Type:        string(task.Type),
	}

	// Workspace may be in Context or Metadata
	if task.Context != nil {
		if ws, ok := task.Context["workspace"]; ok {
			if wsStr, ok := ws.(string); ok {
				view.Workspace = wsStr
			}
		}
	}

	return view
}

// buildContextMap constructs the context alias map from step state.
// It uses the resolver to look up alias values from the state.
func buildContextMap(stepState *core.Context, resolver *AliasResolver) map[string]string {
	if stepState == nil || resolver == nil {
		return make(map[string]string)
	}

	result := make(map[string]string)

	// For each standard alias, try to get the value from state
	for alias, stateKey := range StandardAliases {
		if val, ok := stepState.Get(stateKey); ok && val != nil {
			result[alias] = fmt.Sprintf("%v", val)
		}
	}

	return result
}

// BuildEnrichmentBundle creates an EnrichmentBundle from individual source strings.
func BuildEnrichmentBundle(ast, archaeology, bkc string) EnrichmentBundle {
	return EnrichmentBundle{
		AST:         ast,
		Archaeology: archaeology,
		BKC:         bkc,
	}
}
