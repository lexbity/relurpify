package stages

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// StatePayload retrieves workflow retrieval payload from state.
// This replaces the workflowutil.StatePayload stub.
func StatePayload(state interface{}, key string) []byte {
	if state == nil {
		return nil
	}
	// Handle *contextdata.Envelope
	if env, ok := state.(*contextdata.Envelope); ok {
		if raw, ok := env.GetWorkingValue(key); ok && raw != nil {
			if bytes, ok := raw.([]byte); ok {
				return bytes
			}
			if data, err := json.Marshal(raw); err == nil {
				return data
			}
		}
	}
	return nil
}

func buildStagePrompt(stageName string, task *core.Task, state *contextdata.Envelope, primaryLabel string, primaryValue any, toolNames []string, schema string) string {
	var sections []string
	sections = append(sections, fmt.Sprintf("You are the %s stage of a coding pipeline.", stageName))
	if instruction := taskInstruction(task); instruction != "" {
		sections = append(sections, "Task:\n"+instruction)
	}
	if primary := formatPromptValue(primaryValue); primary != "" {
		sections = append(sections, primaryLabel+":\n"+primary)
	}
	if files := renderContextFiles(task, 2500); files != "" {
		sections = append(sections, "Context files:\n"+files)
	}
	if workflow := workflowRetrievalContext(state); workflow != "" {
		sections = append(sections, "Workflow Retrieval:\n"+workflow)
	}
	if prior := recentPipelineOutputs(state); prior != "" {
		sections = append(sections, "Prior stage outputs:\n"+prior)
	}
	sections = append(sections, "Available tools for this stage: "+renderToolNames(toolNames))
	sections = append(sections, "Return ONLY JSON:\n"+schema)
	return strings.Join(sections, "\n\n")
}

func formatPromptValue(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return strings.TrimSpace(fmt.Sprint(value))
		}
		return string(encoded)
	}
}

func recentPipelineOutputs(state *contextdata.Envelope) string {
	if state == nil {
		return ""
	}
	keys := []string{
		"pipeline.explore",
		"pipeline.analyze",
		"pipeline.plan",
		"pipeline.code",
		"pipeline.verify",
	}
	var sections []string
	for _, key := range keys {
		raw, ok := state.GetWorkingValue(key)
		if !ok || raw == nil {
			continue
		}
		sections = append(sections, fmt.Sprintf("%s:\n%s", key, formatPromptValue(raw)))
	}
	return strings.Join(sections, "\n\n")
}

func workflowRetrievalContext(state *contextdata.Envelope) string {
	if state == nil {
		return ""
	}
	if payload := StatePayload(state, "pipeline.workflow_retrieval"); len(payload) > 0 {
		var data map[string]any
		if err := json.Unmarshal(payload, &data); err == nil {
			if formatted := formatWorkflowRetrievalPromptValue(data); formatted != "" {
				return formatted
			}
		}
	}
	raw, ok := state.GetWorkingValue("pipeline.workflow_retrieval")
	if !ok || raw == nil {
		return ""
	}
	return formatPromptValue(raw)
}

func formatWorkflowRetrievalPromptValue(payload map[string]any) string {
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
		line := fmt.Sprintf("%d. %s", i+1, truncatePromptText(text, 240))
		if ref := workflowRetrievalReference(result); ref != "" {
			line += "\n   Reference: " + ref
		}
		if refs := citationRefs(result["citations"]); len(refs) > 0 {
			if len(refs) > 0 {
				line += "\n   Sources: " + strings.Join(refs, ", ")
			}
		}
		lines = append(lines, line)
	}
	if len(lines) > 0 {
		sections = append(sections, "Evidence:\n"+strings.Join(lines, "\n"))
	}
	return strings.Join(sections, "\n")
}

func citationRefs(raw any) []string {
	switch v := raw.(type) {
	case []string:
		refs := make([]string, 0, len(v))
		for _, item := range v {
			if ref := strings.TrimSpace(item); ref != "" {
				refs = append(refs, ref)
			}
		}
		return refs
	case []any:
		refs := make([]string, 0, len(v))
		for _, item := range v {
			if ref := citationRef(item); ref != "" {
				refs = append(refs, ref)
			}
		}
		return refs
	case map[string]any:
		if ref := citationRef(v); ref != "" {
			return []string{ref}
		}
		return nil
	default:
		return nil
	}
}

func citationRef(raw any) string {
	switch v := raw.(type) {
	case map[string]any:
		return firstPromptValue(
			strings.TrimSpace(fmt.Sprint(v["canonical_uri"])),
			strings.TrimSpace(fmt.Sprint(v["chunk_id"])),
			strings.TrimSpace(fmt.Sprint(v["doc_id"])),
			strings.TrimSpace(fmt.Sprint(v["uri"])),
			strings.TrimSpace(fmt.Sprint(v["id"])),
		)
	default:
		return strings.TrimSpace(fmt.Sprint(raw))
	}
}

func workflowRetrievalReference(result map[string]any) string {
	raw, ok := result["reference"].(map[string]any)
	if !ok || len(raw) == 0 {
		return ""
	}
	return firstPromptValue(
		strings.TrimSpace(fmt.Sprint(raw["uri"])),
		strings.TrimSpace(fmt.Sprint(raw["id"])),
		strings.TrimSpace(fmt.Sprint(raw["detail"])),
	)
}

func truncatePromptText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "..."
}

func firstPromptValue(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func renderToolNames(toolNames []string) string {
	if len(toolNames) == 0 {
		return "none"
	}
	clean := make([]string, 0, len(toolNames))
	seen := make(map[string]struct{}, len(toolNames))
	for _, name := range toolNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		clean = append(clean, name)
	}
	sort.Strings(clean)
	if len(clean) == 0 {
		return "none"
	}
	return strings.Join(clean, ", ")
}
