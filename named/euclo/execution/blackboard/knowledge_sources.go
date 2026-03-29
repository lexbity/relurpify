package blackboard

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	agentblackboard "github.com/lexcodex/relurpify/agents/blackboard"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

func NewAnalysisKnowledgeSource(name, activationPredicate string, tools []string, promptTemplate string) agentblackboard.KnowledgeSource {
	return newTemplateKnowledgeSource(name, activationPredicate, nil, tools, promptTemplate, templateKindAnalysis)
}

func NewMutationKnowledgeSource(name, activationPredicate string, tools []string, promptTemplate string) agentblackboard.KnowledgeSource {
	return newTemplateKnowledgeSource(name, activationPredicate, nil, tools, promptTemplate, templateKindMutation)
}

func NewSynthesisKnowledgeSource(name, activationPredicate string, inputEntries []string, promptTemplate string) agentblackboard.KnowledgeSource {
	return newTemplateKnowledgeSource(name, activationPredicate, inputEntries, nil, promptTemplate, templateKindSynthesis)
}

type templateKind string

const (
	templateKindAnalysis  templateKind = "analysis"
	templateKindMutation  templateKind = "mutation"
	templateKindSynthesis templateKind = "synthesis"
)

type templateKnowledgeSource struct {
	name                string
	activationPredicate string
	inputEntries        []string
	tools               []string
	promptTemplate      string
	kind                templateKind
}

func newTemplateKnowledgeSource(name, activationPredicate string, inputEntries, tools []string, promptTemplate string, kind templateKind) agentblackboard.KnowledgeSource {
	return &templateKnowledgeSource{
		name:                strings.TrimSpace(name),
		activationPredicate: strings.TrimSpace(activationPredicate),
		inputEntries:        append([]string(nil), inputEntries...),
		tools:               append([]string(nil), tools...),
		promptTemplate:      strings.TrimSpace(promptTemplate),
		kind:                kind,
	}
}

func (k *templateKnowledgeSource) Name() string { return firstNonEmpty(k.name, string(k.kind)) }

func (k *templateKnowledgeSource) Priority() int {
	switch k.kind {
	case templateKindAnalysis:
		return 90
	case templateKindMutation:
		return 70
	case templateKindSynthesis:
		return 60
	default:
		return 50
	}
}

func (k *templateKnowledgeSource) CanActivate(bb *agentblackboard.Blackboard) bool {
	if k.activationPredicate == "" {
		if len(k.inputEntries) == 0 {
			return true
		}
		return allBoardEntriesPresent(bb, k.inputEntries)
	}
	return evaluateActivationPredicate(bb, k.activationPredicate)
}

func (k *templateKnowledgeSource) Execute(ctx context.Context, bb *agentblackboard.Blackboard, _ *capability.Registry, model core.LanguageModel) error {
	if model == nil {
		return fmt.Errorf("knowledge source %q requires a language model", k.Name())
	}
	prompt := renderKnowledgeSourcePrompt(k, bb)
	resp, err := model.Generate(ctx, prompt, &core.LLMOptions{Temperature: 0})
	if err != nil {
		return err
	}
	if resp == nil {
		return fmt.Errorf("knowledge source %q returned no response", k.Name())
	}
	if err := applyKnowledgeSourceResponse(bb, k.Name(), resp.Text); err != nil {
		return fmt.Errorf("knowledge source %q: %w", k.Name(), err)
	}
	return nil
}

func (k *templateKnowledgeSource) KnowledgeSourceSpec() agentblackboard.KnowledgeSourceSpec {
	spec := agentblackboard.KnowledgeSourceSpec{
		Name:                 k.Name(),
		Priority:             k.Priority(),
		RequiredCapabilities: selectorsForToolIDs(k.tools),
		Contract:             graph.NodeContract{},
	}
	switch k.kind {
	case templateKindMutation:
		spec.Contract.SideEffectClass = graph.SideEffectHuman
	default:
		spec.Contract.SideEffectClass = graph.SideEffectNone
	}
	return spec
}

func selectorsForToolIDs(toolIDs []string) []core.CapabilitySelector {
	if len(toolIDs) == 0 {
		return nil
	}
	selectors := make([]core.CapabilitySelector, 0, len(toolIDs))
	for _, toolID := range toolIDs {
		toolID = strings.TrimSpace(toolID)
		if toolID == "" {
			continue
		}
		selectors = append(selectors, core.CapabilitySelector{ID: toolID})
	}
	return selectors
}

func allBoardEntriesPresent(bb *agentblackboard.Blackboard, entries []string) bool {
	for _, entry := range entries {
		if !boardHasEntry(bb, entry) {
			return false
		}
	}
	return true
}

func evaluateActivationPredicate(bb *agentblackboard.Blackboard, predicate string) bool {
	predicate = strings.TrimSpace(predicate)
	if predicate == "" || strings.EqualFold(predicate, "always") {
		return true
	}
	if strings.HasPrefix(strings.ToLower(predicate), "not ") {
		return !evaluateActivationPredicate(bb, strings.TrimSpace(predicate[4:]))
	}
	if strings.HasPrefix(predicate, "fact:") {
		return boardHasFactKey(bb, strings.TrimSpace(strings.TrimPrefix(predicate, "fact:")))
	}
	if strings.HasPrefix(predicate, "artifact:") {
		return boardHasArtifactKind(bb, strings.TrimSpace(strings.TrimPrefix(predicate, "artifact:")))
	}
	if strings.HasSuffix(predicate, " exists") {
		return boardHasEntry(bb, strings.TrimSpace(strings.TrimSuffix(predicate, " exists")))
	}
	return boardHasEntry(bb, predicate)
}

func boardHasFactKey(bb *agentblackboard.Blackboard, key string) bool {
	if bb == nil {
		return false
	}
	key = strings.TrimSpace(key)
	for _, fact := range bb.Facts {
		if fact.Key == key {
			return true
		}
	}
	return false
}

func boardHasArtifactKind(bb *agentblackboard.Blackboard, kind string) bool {
	if bb == nil {
		return false
	}
	kind = strings.TrimSpace(kind)
	for _, artifact := range bb.Artifacts {
		if artifact.Kind == kind {
			return true
		}
	}
	return false
}

func renderKnowledgeSourcePrompt(source *templateKnowledgeSource, bb *agentblackboard.Blackboard) string {
	entrySnapshot := map[string]any{}
	for _, fact := range bb.Facts {
		entrySnapshot[fact.Key] = decodeValue(fact.Value)
	}
	for _, artifact := range bb.Artifacts {
		entrySnapshot["artifact:"+artifact.Kind] = decodeValue(artifact.Content)
	}
	goal := ""
	if bb != nil && len(bb.Goals) > 0 {
		goal = bb.Goals[0]
	}
	replacements := map[string]string{
		"{{knowledge_source}}": source.Name(),
		"{{goal}}":             goal,
		"{{available_tools}}":  strings.Join(source.tools, ", "),
		"{{entries}}":          mustJSON(entrySnapshot),
		"{{input_entries}}":    mustJSON(selectedEntries(entrySnapshot, source.inputEntries)),
	}
	prompt := source.promptTemplate
	if prompt == "" {
		prompt = "Goal: {{goal}}\nEntries: {{entries}}\nReturn JSON with optional facts, artifacts, issues, or hypotheses."
	}
	for token, value := range replacements {
		prompt = strings.ReplaceAll(prompt, token, value)
	}
	return prompt
}

func selectedEntries(entries map[string]any, names []string) map[string]any {
	if len(names) == 0 {
		return entries
	}
	selected := make(map[string]any, len(names))
	for _, name := range names {
		if value, ok := entries[name]; ok {
			selected[name] = value
		}
	}
	return selected
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func applyKnowledgeSourceResponse(bb *agentblackboard.Blackboard, sourceName, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("empty response")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return fmt.Errorf("invalid JSON response: %w", err)
	}
	if facts, ok := payload["facts"].([]any); ok {
		for _, item := range facts {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			key := strings.TrimSpace(fmt.Sprint(record["key"]))
			if key == "" || key == "<nil>" {
				continue
			}
			setBoardEntry(bb, key, record["value"], sourceName)
		}
	}
	if hypotheses, ok := payload["hypotheses"].([]any); ok {
		for idx, item := range hypotheses {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id := strings.TrimSpace(fmt.Sprint(record["id"]))
			if id == "" || id == "<nil>" {
				id = fmt.Sprintf("%s-hypothesis-%d", sourceName, idx+1)
			}
			summary := strings.TrimSpace(fmt.Sprint(record["summary"]))
			if summary == "" || summary == "<nil>" {
				summary = strings.TrimSpace(fmt.Sprint(record["title"]))
			}
			bb.Hypotheses = append(bb.Hypotheses, agentblackboard.Hypothesis{
				ID:          id,
				Description: summary,
				Confidence:  floatValue(record["confidence"]),
				Source:      sourceName,
			})
		}
	}
	sort.Slice(bb.Hypotheses, func(i, j int) bool {
		if bb.Hypotheses[i].Confidence == bb.Hypotheses[j].Confidence {
			return bb.Hypotheses[i].ID < bb.Hypotheses[j].ID
		}
		return bb.Hypotheses[i].Confidence > bb.Hypotheses[j].Confidence
	})
	return nil
}

func stringSliceFromAny(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" && s != "<nil>" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func floatValue(raw any) float64 {
	switch typed := raw.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	default:
		return 0
	}
}
