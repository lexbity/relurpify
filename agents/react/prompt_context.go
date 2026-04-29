package react

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkskills "codeburg.org/lexbit/relurpify/framework/skills"
)

// TaskPayload retrieves workflow retrieval payload from task context.
// This replaces the workflowutil.TaskPayload stub.
func TaskPayload(task *core.Task, key string) []byte {
	if task == nil || task.Context == nil {
		return nil
	}
	raw, ok := task.Context[key]
	if !ok || raw == nil {
		return nil
	}
	if bytes, ok := raw.([]byte); ok {
		return bytes
	}
	// Try to marshal if it's not already bytes
	if data, err := json.Marshal(raw); err == nil {
		return data
	}
	return nil
}

type promptContextAssembler struct {
	agent *ReActAgent
	task  *core.Task
}

func newPromptContextAssembler(agent *ReActAgent, task *core.Task) *promptContextAssembler {
	return &promptContextAssembler{agent: agent, task: task}
}

func (a *promptContextAssembler) buildPrompt(state *contextdata.Envelope, tools []core.Tool, compactHistory bool) string {
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
	if memory := a.declarativeMemory(state); memory != "" {
		sections = append(sections, "Relevant Memory:\n"+memory)
	}
	if workflow := a.workflowRetrieval(); workflow != "" {
		sections = append(sections, "Workflow Retrieval:\n"+workflow)
	}
	if streamed := a.streamedContext(state); streamed != "" {
		sections = append(sections, "Streamed Context:\n"+streamed)
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

func (a *promptContextAssembler) planGoal(state *contextdata.Envelope) string {
	if state == nil {
		return ""
	}
	raw, ok := envelopeGet(state, "architect.plan")
	if !ok {
		raw, ok = envelopeGet(state, "planner.plan")
	}
	if !ok || raw == nil {
		return ""
	}
	plan, ok := raw.(agentgraph.Plan)
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
	if step, ok := raw.(agentgraph.PlanStep); ok {
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

func (a *promptContextAssembler) previousStepSummary(state *contextdata.Envelope) string {
	if state == nil {
		return ""
	}
	return strings.TrimSpace(envelopeGetString(state, "architect.last_step_summary"))
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

func (a *promptContextAssembler) declarativeMemory(state *contextdata.Envelope) string {
	if state == nil {
		return ""
	}
	if raw, ok := envelopeGet(state, "graph.declarative_memory_payload"); ok && raw != nil {
		if payload, ok := raw.(map[string]any); ok {
			if formatted := formatMemoryRetrievalPayload(payload); formatted != "" {
				return formatted
			}
		}
	}
	if raw, ok := envelopeGet(state, "graph.declarative_memory_refs"); ok && raw != nil {
		if refs, ok := raw.([]agentgraph.ContextReference); ok {
			if formatted := formatMemoryReferenceList(refs); formatted != "" {
				return formatted
			}
		}
	}
	raw, ok := envelopeGet(state, "graph.declarative_memory")
	if !ok || raw == nil {
		return ""
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	results, _ := payload["results"].([]core.MemoryRecordEnvelope)
	if len(results) == 0 {
		return ""
	}
	var parts []string
	for _, r := range results {
		if summary := strings.TrimSpace(r.Summary); summary != "" {
			parts = append(parts, "- "+summary)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

func formatMemoryRetrievalPayload(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	results, ok := payload["results"].([]map[string]any)
	if !ok || len(results) == 0 {
		return ""
	}
	parts := make([]string, 0, len(results))
	for _, result := range results {
		text := strings.TrimSpace(fmt.Sprint(result["summary"]))
		if text == "" || text == "<nil>" {
			text = strings.TrimSpace(fmt.Sprint(result["text"]))
		}
		if text == "" || text == "<nil>" {
			continue
		}
		if source := strings.TrimSpace(fmt.Sprint(result["source"])); source != "" && source != "<nil>" {
			text += " [" + source + "]"
		}
		parts = append(parts, "- "+text)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

func formatMemoryReferenceList(refs []agentgraph.ContextReference) string {
	if len(refs) == 0 {
		return ""
	}
	lines := make([]string, 0, len(refs))
	for _, ref := range refs {
		label := strings.TrimSpace(ref.URI)
		if label == "" {
			label = strings.TrimSpace(ref.ID)
		}
		if label == "" {
			continue
		}
		line := "- Reference: " + label
		if ref.Kind != "" {
			line += fmt.Sprintf(" (%s)", ref.Kind)
		}
		if ref.Detail != "" {
			line += " [" + strings.TrimSpace(ref.Detail) + "]"
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func (a *promptContextAssembler) streamedContext(state *contextdata.Envelope) string {
	if state == nil || len(state.References.StreamedContext) == 0 {
		return ""
	}
	lines := make([]string, 0, len(state.References.StreamedContext))
	for _, ref := range state.References.StreamedContext {
		chunkID := strings.TrimSpace(string(ref.ChunkID))
		if chunkID == "" {
			continue
		}
		line := "- " + chunkID
		if ref.Source != "" {
			line += " [" + strings.TrimSpace(ref.Source) + "]"
		}
		if ref.Rank > 0 {
			line += fmt.Sprintf(" rank=%d", ref.Rank)
		}
		if ref.IsSummary {
			line += " summary"
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func (a *promptContextAssembler) workflowRetrieval() string {
	if a == nil || a.task == nil || a.task.Context == nil {
		return ""
	}
	if payload := TaskPayload(a.task, "workflow_retrieval"); len(payload) > 0 {
		var data map[string]any
		if err := json.Unmarshal(payload, &data); err == nil {
			if formatted := formatWorkflowRetrievalPayload(data); formatted != "" {
				return formatted
			}
		}
	}
	raw, ok := a.task.Context["workflow_retrieval"]
	if !ok || raw == nil {
		return ""
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
			text = strings.TrimSpace(fmt.Sprint(result["summary"]))
		}
		if text == "" || text == "<nil>" {
			text = "reference only"
		}
		line := fmt.Sprintf("%d. %s", i+1, truncateWorkflowEvidence(text, 240))
		if ref := workflowRetrievalReference(result); ref != "" {
			line += "\n   Reference: " + ref
		}
		// Citations temporarily disabled - retrieval package being rebuilt
		if citations, ok := result["citations"].([]any); ok && len(citations) > 0 {
			_ = citations
		}
		// Add anchor notices for drifted or superseded anchors
		if anchors, ok := result["anchors"].([]any); ok {
			anchorNotices := buildAnchorNotices(anchors)
			if anchorNotices != "" {
				line += "\n" + anchorNotices
			}
		}
		// Add derivation depth warning if evidence is heavily transformed
		if derivation, ok := result["derivation"].(map[string]any); ok {
			depthWarning := buildDerivationDepthWarning(derivation)
			if depthWarning != "" {
				line += "\n" + depthWarning
			}
		}
		lines = append(lines, line)
	}
	if len(lines) > 0 {
		sections = append(sections, "Evidence:\n"+strings.Join(lines, "\n"))
	}
	return strings.Join(sections, "\n")
}

func workflowRetrievalReference(result map[string]any) string {
	raw, ok := result["reference"].(map[string]any)
	if !ok || len(raw) == 0 {
		return ""
	}
	return firstWorkflowSource(
		strings.TrimSpace(fmt.Sprint(raw["uri"])),
		strings.TrimSpace(fmt.Sprint(raw["id"])),
		strings.TrimSpace(fmt.Sprint(raw["detail"])),
	)
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

func (a *promptContextAssembler) currentPhase(state *contextdata.Envelope) string {
	if state == nil {
		return ""
	}
	return strings.TrimSpace(envelopeGetString(state, "react.phase"))
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

func (a *promptContextAssembler) compactHistory(state *contextdata.Envelope, compact bool) string {
	if state == nil {
		return ""
	}
	if !compact {
		return envelopeGetContextForLLM(state)
	}
	var sections []string
	compressed, history := envelopeGetFullHistory(state)
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

func (a *promptContextAssembler) contextFiles(state *contextdata.Envelope) string {
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

func (a *promptContextAssembler) recentToolObservations(state *contextdata.Envelope) string {
	if state == nil {
		return ""
	}
	raw, ok := envelopeGet(state, "react.tool_observations")
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

// buildAnchorNotices formats warning notices for drifted or superseded anchors in evidence.
func buildAnchorNotices(anchorsData []any) string {
	if len(anchorsData) == 0 {
		return ""
	}

	var notices []string
	for _, anchorAny := range anchorsData {
		anchorMap, ok := anchorAny.(map[string]any)
		if !ok {
			continue
		}

		term := fmt.Sprint(anchorMap["term"])
		definition := fmt.Sprint(anchorMap["definition"])
		status := fmt.Sprint(anchorMap["status"])

		// Only show notices for non-fresh anchors
		if status == "fresh" || status == "" {
			continue
		}

		var notice string
		switch status {
		case "drifted":
			notice = fmt.Sprintf("⚠ ANCHOR DRIFT: \"%s\" was defined as \"%s\" when this evidence was captured. The surrounding context has since changed.", term, definition)
		case "superseded":
			notice = fmt.Sprintf("⚠ ANCHOR SUPERSEDED: \"%s\" is no longer the active definition. This evidence uses an outdated term.", term)
		}

		if notice != "" {
			notices = append(notices, "   "+notice)
		}
	}

	if len(notices) > 0 {
		return strings.Join(notices, "\n")
	}
	return ""
}

// buildDerivationDepthWarning formats a confidence warning for heavily transformed evidence.
func buildDerivationDepthWarning(derivation map[string]any) string {
	const (
		depthThreshold = 4
		lossThreshold  = 0.5
	)

	depth := toInt(derivation["depth"])
	totalLoss := toFloat(derivation["total_loss"])
	originSystem := fmt.Sprint(derivation["origin_system"])

	// Check if depth or loss exceeds thresholds
	if depth <= depthThreshold && totalLoss <= lossThreshold {
		return ""
	}

	lossPercent := int(totalLoss * 100)
	return fmt.Sprintf("   ⚠ CONFIDENCE: This evidence has been through %d transformations with estimated %d%% information loss. Origin: %s",
		depth, lossPercent, originSystem)
}

// toInt safely converts any to int.
func toInt(v any) int {
	if v == nil {
		return 0
	}
	switch typed := v.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	case string:
		i, _ := strconv.Atoi(typed)
		return i
	default:
		return 0
	}
}

// toFloat safely converts any to float64.
func toFloat(v any) float64 {
	if v == nil {
		return 0.0
	}
	switch typed := v.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case string:
		f, _ := strconv.ParseFloat(typed, 64)
		return f
	default:
		return 0.0
	}
}
