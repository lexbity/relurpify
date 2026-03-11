package pattern

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/retrieval"
	frameworkskills "github.com/lexcodex/relurpify/framework/skills"
)

type promptContextAssembler struct {
	agent *ReActAgent
	task  *core.Task
}

func newPromptContextAssembler(agent *ReActAgent, task *core.Task) *promptContextAssembler {
	return &promptContextAssembler{agent: agent, task: task}
}

func (a *promptContextAssembler) buildPrompt(state *core.Context, tools []core.Tool, compactHistory bool) string {
	if a == nil || a.task == nil {
		return ""
	}
	var sections []string
	sections = append(sections, fmt.Sprintf("Task: %s", a.task.Instruction))
	if goal := a.planGoal(state); goal != "" {
		sections = append(sections, "Plan Goal:\n"+goal)
	}
	if step := a.currentStep(); step != "" {
		sections = append(sections, "Current Step:\n"+step)
	}
	if previous := a.previousStepSummary(state); previous != "" {
		sections = append(sections, "Previous Step Result:\n"+previous)
	}
	if external := a.externalStateSlice(); external != "" {
		sections = append(sections, "External State Slice:\n"+external)
	}
	if workflow := a.workflowRetrieval(); workflow != "" {
		sections = append(sections, "Workflow Retrieval:\n"+workflow)
	}
	if phase := a.currentPhase(state); phase != "" {
		sections = append(sections, "Execution Phase:\n"+phase)
	}
	if hints := a.skillHints(); hints != "" {
		sections = append(sections, "Skill Policy:\n"+hints)
	}
	if catalog := a.capabilityCatalog(); catalog != "" {
		sections = append(sections, "Capability Catalog:\n"+catalog)
	}
	if files := a.contextFiles(state); files != "" {
		sections = append(sections, "Working Context:\n"+files)
	}
	if observations := a.recentToolObservations(state); observations != "" {
		sections = append(sections, "Recent Observations:\n"+observations)
	}
	if history := a.compactHistory(state, compactHistory); history != "" {
		sections = append(sections, "Relevant History:\n"+history)
	}
	sections = append(sections, fmt.Sprintf("Available tools in this phase: %s", strings.Join(toolNames(tools), ", ")))
	return strings.Join(sections, "\n\n")
}

func (a *promptContextAssembler) skillHints() string {
	if a == nil || a.task == nil {
		return ""
	}
	effective := frameworkskills.ResolveEffectiveSkillPolicy(a.task, a.agent.effectiveAgentSpec(a.task), a.agent.Tools)
	spec := effective.Spec
	if spec == nil {
		return ""
	}
	return frameworkskills.RenderExecutionPolicy(&effective.Policy, spec.SkillConfig.Verification.StopOnSuccess)
}

func (a *promptContextAssembler) planGoal(state *core.Context) string {
	if state == nil {
		return ""
	}
	raw, ok := state.Get("architect.plan")
	if !ok {
		raw, ok = state.Get("planner.plan")
	}
	if !ok || raw == nil {
		return ""
	}
	plan, ok := raw.(core.Plan)
	if !ok {
		return ""
	}
	if plan.Goal != "" {
		return plan.Goal
	}
	if len(plan.Files) == 0 {
		return ""
	}
	return "Files in scope: " + strings.Join(plan.Files, ", ")
}

func (a *promptContextAssembler) currentStep() string {
	if a.task == nil || a.task.Context == nil {
		return ""
	}
	raw, ok := a.task.Context["current_step"]
	if !ok || raw == nil {
		return ""
	}
	if step, ok := raw.(core.PlanStep); ok {
		encoded, err := json.MarshalIndent(step, "", "  ")
		if err == nil {
			return string(encoded)
		}
		return step.Description
	}
	encoded, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Sprint(raw)
	}
	return string(encoded)
}

func (a *promptContextAssembler) previousStepSummary(state *core.Context) string {
	if state == nil {
		return ""
	}
	return strings.TrimSpace(state.GetString("architect.last_step_summary"))
}

func (a *promptContextAssembler) externalStateSlice() string {
	if a == nil || a.task == nil || a.task.Context == nil {
		return ""
	}
	raw, ok := a.task.Context["external_state_slice"]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		encoded, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			return strings.TrimSpace(fmt.Sprint(raw))
		}
		return string(encoded)
	}
}

func (a *promptContextAssembler) workflowRetrieval() string {
	if a == nil || a.task == nil || a.task.Context == nil {
		return ""
	}
	raw, ok := a.task.Context["workflow_retrieval"]
	if !ok || raw == nil {
		return ""
	}
	if payload, ok := raw.(map[string]any); ok {
		if formatted := formatWorkflowRetrievalPayload(payload); formatted != "" {
			return formatted
		}
	}
	encoded, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return strings.TrimSpace(fmt.Sprint(raw))
	}
	return string(encoded)
}

func formatWorkflowRetrievalPayload(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	var sections []string
	if query := strings.TrimSpace(fmt.Sprint(payload["query"])); query != "" && query != "<nil>" {
		sections = append(sections, "Query: "+query)
	}
	if scope := strings.TrimSpace(fmt.Sprint(payload["scope"])); scope != "" && scope != "<nil>" {
		sections = append(sections, "Scope: "+scope)
	}
	if cacheTier := strings.TrimSpace(fmt.Sprint(payload["cache_tier"])); cacheTier != "" && cacheTier != "<nil>" {
		sections = append(sections, "Cache tier: "+cacheTier)
	}
	results, ok := payload["results"].([]map[string]any)
	if !ok || len(results) == 0 {
		return strings.Join(sections, "\n")
	}
	lines := make([]string, 0, len(results))
	for i, result := range results {
		text := strings.TrimSpace(fmt.Sprint(result["text"]))
		if text == "" || text == "<nil>" {
			continue
		}
		line := fmt.Sprintf("%d. %s", i+1, truncateWorkflowEvidence(text, 240))
		if citations, ok := result["citations"].([]retrieval.PackedCitation); ok && len(citations) > 0 {
			sources := make([]string, 0, len(citations))
			for _, citation := range citations {
				source := firstWorkflowSource(citation.CanonicalURI, citation.ChunkID, citation.DocID)
				if source == "" {
					continue
				}
				sources = append(sources, source)
			}
			if len(sources) > 0 {
				line += "\n   Sources: " + strings.Join(sources, ", ")
			}
		}
		lines = append(lines, line)
	}
	if len(lines) > 0 {
		sections = append(sections, "Evidence:\n"+strings.Join(lines, "\n"))
	}
	return strings.Join(sections, "\n")
}

func truncateWorkflowEvidence(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "..."
}

func firstWorkflowSource(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (a *promptContextAssembler) currentPhase(state *core.Context) string {
	if state == nil {
		return ""
	}
	return strings.TrimSpace(state.GetString("react.phase"))
}

func (a *promptContextAssembler) capabilityCatalog() string {
	if a == nil || a.agent == nil || a.agent.Tools == nil {
		return ""
	}
	capabilities := a.agent.Tools.AllCapabilities()
	if len(capabilities) == 0 {
		return ""
	}
	var lines []string
	for _, capability := range capabilities {
		if capability.Kind == core.CapabilityKindTool {
			continue
		}
		label := strings.TrimSpace(capability.Name)
		if label == "" {
			label = capability.ID
		}
		desc := strings.TrimSpace(capability.Description)
		if desc == "" {
			desc = string(capability.Kind)
		}
		lines = append(lines, fmt.Sprintf("- %s [%s]: %s", label, capability.Kind, truncateForPrompt(desc, 120)))
	}
	if len(lines) == 0 {
		return ""
	}
	sort.Strings(lines)
	if len(lines) > 6 {
		lines = lines[:6]
	}
	return strings.Join(lines, "\n")
}

func (a *promptContextAssembler) compactHistory(state *core.Context, compact bool) string {
	if state == nil {
		return ""
	}
	if !compact {
		return strings.TrimSpace(state.GetContextForLLM())
	}
	var sections []string
	compressed, history := state.GetFullHistory()
	if len(compressed) > 0 {
		latest := compressed[len(compressed)-1]
		if latest.Summary != "" {
			sections = append(sections, "Compressed summary: "+latest.Summary)
		}
	}
	if len(history) > 0 {
		start := 0
		if len(history) > 4 {
			start = len(history) - 4
		}
		var turns []string
		for _, interaction := range history[start:] {
			turns = append(turns, fmt.Sprintf("[%s] %s", interaction.Role, truncateForPrompt(interaction.Content, 160)))
		}
		sections = append(sections, strings.Join(turns, "\n"))
	}
	return strings.Join(sections, "\n")
}

func (a *promptContextAssembler) contextFiles(state *core.Context) string {
	if a == nil || a.agent == nil || a.agent.contextPolicy == nil || a.agent.contextPolicy.ContextManager == nil {
		return renderContextFiles(a.task, 3000)
	}
	items := a.agent.contextPolicy.ContextManager.GetItems()
	if len(items) == 0 {
		return renderContextFiles(a.task, 3000)
	}
	type scoredItem struct {
		score float64
		text  string
	}
	var scored []scoredItem
	for _, item := range items {
		switch typed := item.(type) {
		case *core.FileContextItem:
			content := typed.Content
			if content == "" {
				content = typed.Summary
			}
			if strings.TrimSpace(content) == "" {
				continue
			}
			scored = append(scored, scoredItem{
				score: typed.RelevanceScore(),
				text:  fmt.Sprintf("File: %s\n%s", typed.Path, truncateForPrompt(content, 1000)),
			})
		case *core.ToolResultContextItem:
			payload := typed.Result
			if payload == nil {
				continue
			}
			text, ok := renderInsertionFilteredSummary(a.agent, a.task, typed.ToolName, payload, typed.Envelope)
			if !ok || strings.TrimSpace(text) == "" {
				continue
			}
			scored = append(scored, scoredItem{
				score: typed.RelevanceScore(),
				text:  fmt.Sprintf("Tool %s: %s", typed.ToolName, text),
			})
		}
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})
	var blocks []string
	remaining := 2200
	for _, item := range scored {
		if remaining <= 0 {
			break
		}
		text := item.text
		if len(text) > remaining {
			text = text[:remaining]
		}
		blocks = append(blocks, text)
		remaining -= len(text)
	}
	return strings.Join(blocks, "\n\n")
}

func (a *promptContextAssembler) recentToolObservations(state *core.Context) string {
	if state == nil {
		return ""
	}
	raw, ok := state.Get("react.tool_observations")
	if !ok || raw == nil {
		return ""
	}
	observations, ok := raw.([]ToolObservation)
	if !ok || len(observations) == 0 {
		return ""
	}
	start := 0
	if len(observations) > 4 {
		start = len(observations) - 4
	}
	lines := make([]string, 0, len(observations[start:]))
	for _, obs := range observations[start:] {
		lines = append(lines, fmt.Sprintf("- %s", obs.Summary))
	}
	return strings.Join(lines, "\n")
}

func toolNames(tools []core.Tool) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.Name())
	}
	sort.Strings(out)
	return out
}

func summarizeToolPayload(result *core.ToolResult) string {
	if result == nil {
		return ""
	}
	if summary, ok := result.Data["summary"].(string); ok && summary != "" {
		return summary
	}
	if result.Error != "" {
		return result.Error
	}
	return truncateForPrompt(fmt.Sprint(result.Data), 220)
}

func truncateForPrompt(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return strings.TrimSpace(value[:max]) + "..."
}

func toolSummaryBudgetForPhase(phase string) int {
	switch phase {
	case contextmgrPhaseVerify:
		return 6
	case contextmgrPhaseEdit:
		return 4
	default:
		return 5
	}
}
