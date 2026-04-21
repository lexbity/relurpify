package execution

import (
	"encoding/json"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

func MissingPlanSymbols(step *frameworkplan.PlanStep, graph *graphdb.Engine) []string {
	if step == nil || graph == nil {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, symbolID := range append(append([]string{}, step.Scope...), RequiredSymbols(step)...) {
		if strings.TrimSpace(symbolID) == "" {
			continue
		}
		if _, ok := seen[symbolID]; ok {
			continue
		}
		seen[symbolID] = struct{}{}
		if _, ok := graph.GetNode(symbolID); !ok {
			out = append(out, symbolID)
		}
	}
	return out
}

func CorpusScopeForTask(task *core.Task) string {
	if task != nil && task.Context != nil {
		if value := strings.TrimSpace(stringValue(task.Context["corpus_scope"])); value != "" {
			return value
		}
	}
	return "workspace"
}

func MixedEvidenceForStep(state *core.Context, step *frameworkplan.PlanStep) (retrieval.MixedEvidenceResult, bool) {
	if state == nil || step == nil {
		return retrieval.MixedEvidenceResult{}, false
	}
	raw, ok := state.Get("pipeline.workflow_retrieval")
	if !ok || raw == nil {
		return retrieval.MixedEvidenceResult{}, false
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		return retrieval.MixedEvidenceResult{}, false
	}
	rawResults, ok := payload["results"].([]any)
	if !ok {
		if typed, ok := payload["results"].([]map[string]any); ok {
			rawResults = make([]any, 0, len(typed))
			for _, item := range typed {
				rawResults = append(rawResults, item)
			}
		} else {
			return retrieval.MixedEvidenceResult{}, false
		}
	}
	results := make([]retrieval.MixedEvidenceResult, 0, len(rawResults))
	for _, item := range rawResults {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		data, err := json.Marshal(itemMap)
		if err != nil {
			continue
		}
		var result retrieval.MixedEvidenceResult
		if err := json.Unmarshal(data, &result); err != nil {
			continue
		}
		results = append(results, result)
	}
	if len(results) == 0 {
		return retrieval.MixedEvidenceResult{}, false
	}
	requiredAnchorSet := make(map[string]struct{})
	for _, anchorID := range step.AnchorDependencies {
		requiredAnchorSet[anchorID] = struct{}{}
	}
	if step.EvidenceGate != nil {
		for _, anchorID := range step.EvidenceGate.RequiredAnchors {
			requiredAnchorSet[anchorID] = struct{}{}
		}
	}
	for _, result := range results {
		for _, anchor := range result.Anchors {
			if _, ok := requiredAnchorSet[anchor.AnchorID]; ok {
				return result, true
			}
		}
	}
	return results[0], true
}

func AvailableSymbolMap(step *frameworkplan.PlanStep, graph *graphdb.Engine) map[string]bool {
	symbols := make(map[string]bool)
	if step == nil {
		return symbols
	}
	for _, symbolID := range append(append([]string{}, step.Scope...), RequiredSymbols(step)...) {
		if strings.TrimSpace(symbolID) == "" {
			continue
		}
		if graph == nil {
			symbols[symbolID] = true
			continue
		}
		_, ok := graph.GetNode(symbolID)
		symbols[symbolID] = ok
	}
	return symbols
}

func RequiredSymbols(step *frameworkplan.PlanStep) []string {
	if step == nil || step.EvidenceGate == nil {
		return nil
	}
	return step.EvidenceGate.RequiredSymbols
}

func stringValue(raw any) string {
	if raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	return ""
}
