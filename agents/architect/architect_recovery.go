package architect

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	reactpkg "github.com/lexcodex/relurpify/agents/react"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

func (a *ArchitectAgent) diagnoseStepFailure(ctx context.Context, step core.PlanStep, err error) (string, error) {
	if err == nil {
		return "", nil
	}
	if a == nil || a.Model == nil {
		return diagnoseStepFailure(ctx, step, err)
	}
	prompt := fmt.Sprintf(`Summarize a recovery action for a failed implementation step.
Return one short paragraph with a concrete next action.
Step: %s
Files: %v
Error: %v`, step.Description, step.Files, err)
	resp, genErr := a.Model.Generate(ctx, prompt, &core.LLMOptions{
		Model:       a.Config.Model,
		Temperature: 0,
		MaxTokens:   160,
	})
	if genErr != nil {
		return diagnoseStepFailure(ctx, step, err)
	}
	return strings.TrimSpace(resp.Text), nil
}

func (a *ArchitectAgent) recoverStepFailure(ctx context.Context, step core.PlanStep, stepTask *core.Task, state *core.Context, err error) (*graph.StepRecovery, error) {
	if err == nil {
		return nil, nil
	}
	recoveryNotes, recoveryContext := a.runRecoveryMiniLoop(ctx, step, state, err)
	recovery := &graph.StepRecovery{
		Diagnosis: fmt.Sprintf("Step %s failed and needs a narrower retry.", step.ID),
		Notes:     append(buildRecoveryNotes(step, state, err), recoveryNotes...),
		Context:   recoveryContext,
	}
	if a != nil && a.Model != nil {
		prompt := fmt.Sprintf(`Produce a compact recovery plan for one failed coding step.
Return JSON as {"diagnosis":"...","notes":["..."]}.
Step: %s
Files: %v
Previous step result: %s
Last error: %v`, step.Description, step.Files, state.GetString("architect.last_step_summary"), err)
		resp, genErr := a.Model.Generate(ctx, prompt, &core.LLMOptions{
			Model:       a.Config.Model,
			Temperature: 0,
			MaxTokens:   220,
		})
		if genErr == nil {
			var parsed struct {
				Diagnosis string   `json:"diagnosis"`
				Notes     []string `json:"notes"`
			}
			if jsonErr := json.Unmarshal([]byte(reactpkg.ExtractJSON(resp.Text)), &parsed); jsonErr == nil {
				if strings.TrimSpace(parsed.Diagnosis) != "" {
					recovery.Diagnosis = strings.TrimSpace(parsed.Diagnosis)
				}
				if len(parsed.Notes) > 0 {
					recovery.Notes = append([]string{}, parsed.Notes...)
				}
			}
		}
	}
	if recovery.Context == nil {
		recovery.Context = map[string]any{}
	}
	if len(step.Files) > 0 {
		recovery.Context["recovery_files"] = append([]string{}, step.Files...)
	}
	if stepTask != nil && stepTask.Context != nil {
		if current, ok := stepTask.Context["current_step"]; ok {
			recovery.Context["recovery_step"] = current
		}
	}
	if state != nil {
		state.Set("architect.last_recovery_notes", append([]string{}, recovery.Notes...))
		state.Set("architect.last_recovery_diagnosis", recovery.Diagnosis)
	}
	return recovery, nil
}

func buildRecoveryNotes(step core.PlanStep, state *core.Context, err error) []string {
	notes := []string{
		fmt.Sprintf("Re-check the failing step goal: %s", step.Description),
		fmt.Sprintf("Validate the reported error before editing again: %v", err),
	}
	if len(step.Files) > 0 {
		notes = append(notes, fmt.Sprintf("Inspect the step files first: %s", strings.Join(step.Files, ", ")))
	}
	if state != nil {
		if previous := strings.TrimSpace(state.GetString("architect.last_step_summary")); previous != "" {
			notes = append(notes, "Use the previous step summary as a constraint: "+previous)
		}
	}
	return notes
}

func (a *ArchitectAgent) runRecoveryMiniLoop(ctx context.Context, step core.PlanStep, state *core.Context, err error) ([]string, map[string]any) {
	tools := a.recoveryRegistry()
	if tools == nil {
		return nil, nil
	}
	notes := make([]string, 0, 4)
	evidence := make(map[string]any)

	for _, path := range limitStrings(uniqueStrings(step.Files), 2) {
		if result, execErr := a.executeRecoveryTool(ctx, tools, state, "file_read", map[string]any{"path": path}); execErr == nil && result != nil && result.Success {
			content := strings.TrimSpace(fmt.Sprint(result.Data["content"]))
			snippet := firstRecoverySnippet(content)
			if snippet != "" {
				notes = append(notes, fmt.Sprintf("Inspected %s: %s", path, snippet))
				appendRecoveryEvidence(evidence, "file_reads", map[string]any{"path": path, "snippet": snippet})
			}
		}
	}

	if pattern := recoverySearchPattern(err); pattern != "" {
		args := map[string]any{"pattern": pattern}
		if dir := recoverySearchDirectory(step.Files); dir != "" {
			args["directory"] = dir
		}
		if result, execErr := a.executeRecoveryTool(ctx, tools, state, "search_grep", args); execErr == nil && result != nil && result.Success {
			matches := countRecoveryMatches(result.Data["matches"])
			if matches > 0 {
				notes = append(notes, fmt.Sprintf("Found %d matching lines for %q during recovery.", matches, pattern))
				appendRecoveryEvidence(evidence, "grep", map[string]any{"pattern": pattern, "matches": matches})
			}
		} else if result, execErr := a.executeRecoveryTool(ctx, tools, state, "file_search", args); execErr == nil && result != nil && result.Success {
			matches := countRecoveryMatches(result.Data["matches"])
			if matches > 0 {
				notes = append(notes, fmt.Sprintf("Found %d matching lines for %q during recovery.", matches, pattern))
				appendRecoveryEvidence(evidence, "grep", map[string]any{"pattern": pattern, "matches": matches})
			}
		}
	}

	for _, symbol := range limitStrings(uniqueStrings(extractRecoverySymbols(step)), 1) {
		if result, execErr := a.executeRecoveryTool(ctx, tools, state, "query_ast", map[string]any{"action": "get_signature", "symbol": symbol}); execErr == nil && result != nil && result.Success {
			signature := strings.TrimSpace(fmt.Sprint(result.Data["signature"]))
			if signature != "" {
				notes = append(notes, fmt.Sprintf("AST signature for %s: %s", symbol, truncateRecovery(signature, 120)))
				appendRecoveryEvidence(evidence, "ast", map[string]any{"symbol": symbol, "signature": signature})
			}
		}
	}

	if len(notes) == 0 && len(evidence) == 0 {
		return nil, nil
	}
	return uniqueStrings(notes), evidence
}

func (a *ArchitectAgent) recoveryRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	if a.ExecutorTools != nil {
		return a.ExecutorTools
	}
	return a.PlannerTools
}

func (a *ArchitectAgent) executeRecoveryTool(ctx context.Context, registry *capability.Registry, state *core.Context, name string, args map[string]any) (*core.ToolResult, error) {
	if registry == nil {
		return nil, nil
	}
	if !registry.HasCapability(name) {
		return nil, nil
	}
	if !registry.CapabilityAvailable(ctx, state, name) {
		return nil, nil
	}
	return registry.InvokeCapability(ctx, state, name, args)
}

func appendRecoveryEvidence(evidence map[string]any, key string, value any) {
	if evidence == nil || key == "" || value == nil {
		return
	}
	current, ok := evidence[key]
	if !ok {
		evidence[key] = []any{value}
		return
	}
	switch values := current.(type) {
	case []any:
		evidence[key] = append(values, value)
	default:
		evidence[key] = []any{values, value}
	}
}

func firstRecoverySnippet(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return truncateRecovery(line, 120)
	}
	return ""
}

func truncateRecovery(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return strings.TrimSpace(value[:max]) + "..."
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
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

func limitStrings(values []string, max int) []string {
	if max <= 0 || len(values) <= max {
		return append([]string{}, values...)
	}
	return append([]string{}, values[:max]...)
}

func diagnoseStepFailure(_ context.Context, step core.PlanStep, err error) (string, error) {
	if err == nil {
		return "", nil
	}
	return fmt.Sprintf("Retry step %s with a narrower change set and validate the failing file first. Last error: %v", step.ID, err), nil
}

func countRecoveryMatches(raw any) int {
	switch matches := raw.(type) {
	case []map[string]interface{}:
		return len(matches)
	case []any:
		return len(matches)
	default:
		return 0
	}
}

func recoverySearchPattern(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	pattern := strings.TrimSpace(lines[0])
	if idx := strings.Index(pattern, ":"); idx > 0 {
		rest := strings.TrimSpace(pattern[idx+1:])
		if rest != "" {
			pattern = rest
		}
	}
	if len(pattern) > 80 {
		pattern = pattern[:80]
	}
	return strings.TrimSpace(pattern)
}

func recoverySearchDirectory(files []string) string {
	if len(files) == 0 {
		return "."
	}
	dir := filepath.Dir(files[0])
	if dir == "." || dir == "" {
		return "."
	}
	return dir
}

func extractRecoverySymbols(step core.PlanStep) []string {
	return contextmgr.ExtractSymbolReferences(step.Description + " " + strings.Join(step.Files, " "))
}
