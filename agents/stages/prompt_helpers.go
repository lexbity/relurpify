package stages

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
)

func buildStagePrompt(stageName string, task *core.Task, state *core.Context, primaryLabel string, primaryValue any, toolNames []string, schema string) string {
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

func recentPipelineOutputs(state *core.Context) string {
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
		raw, ok := state.Get(key)
		if !ok || raw == nil {
			continue
		}
		sections = append(sections, fmt.Sprintf("%s:\n%s", key, formatPromptValue(raw)))
	}
	return strings.Join(sections, "\n\n")
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
