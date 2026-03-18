package capabilities

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/agents/blackboard"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

func NewAnalysisKnowledgeSource(name, activationPredicate string, tools []string, promptTemplate string) blackboard.KnowledgeSource {
	return newTemplateKnowledgeSource(name, activationPredicate, nil, tools, promptTemplate, templateKindAnalysis)
}

func NewMutationKnowledgeSource(name, activationPredicate string, tools []string, promptTemplate string) blackboard.KnowledgeSource {
	return newTemplateKnowledgeSource(name, activationPredicate, nil, tools, promptTemplate, templateKindMutation)
}

func NewSynthesisKnowledgeSource(name, activationPredicate string, inputEntries []string, promptTemplate string) blackboard.KnowledgeSource {
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

func newTemplateKnowledgeSource(name, activationPredicate string, inputEntries, tools []string, promptTemplate string, kind templateKind) blackboard.KnowledgeSource {
	return &templateKnowledgeSource{
		name:                strings.TrimSpace(name),
		activationPredicate: strings.TrimSpace(activationPredicate),
		inputEntries:        append([]string(nil), inputEntries...),
		tools:               append([]string(nil), tools...),
		promptTemplate:      strings.TrimSpace(promptTemplate),
		kind:                kind,
	}
}

func (k *templateKnowledgeSource) Name() string {
	return firstNonEmpty(k.name, string(k.kind))
}

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

func (k *templateKnowledgeSource) CanActivate(bb *blackboard.Blackboard) bool {
	if k.activationPredicate == "" {
		if len(k.inputEntries) == 0 {
			return true
		}
		return allBoardEntriesPresent(bb, k.inputEntries)
	}
	return evaluateActivationPredicate(bb, k.activationPredicate)
}

func (k *templateKnowledgeSource) Execute(ctx context.Context, bb *blackboard.Blackboard, _ *capability.Registry, model core.LanguageModel) error {
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

func (k *templateKnowledgeSource) KnowledgeSourceSpec() blackboard.KnowledgeSourceSpec {
	spec := blackboard.KnowledgeSourceSpec{
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

func allBoardEntriesPresent(bb *blackboard.Blackboard, entries []string) bool {
	for _, entry := range entries {
		if !boardHasEntry(bb, entry) {
			return false
		}
	}
	return true
}

func evaluateActivationPredicate(bb *blackboard.Blackboard, predicate string) bool {
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

func boardHasFactKey(bb *blackboard.Blackboard, key string) bool {
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

func boardHasArtifactKind(bb *blackboard.Blackboard, kind string) bool {
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

func renderKnowledgeSourcePrompt(source *templateKnowledgeSource, bb *blackboard.Blackboard) string {
	entrySnapshot := map[string]any{}
	for _, fact := range bb.Facts {
		entrySnapshot[fact.Key] = decodeBridgeValue(fact.Value)
	}
	for _, artifact := range bb.Artifacts {
		entrySnapshot["artifact:"+artifact.Kind] = decodeBridgeValue(artifact.Content)
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

func applyKnowledgeSourceResponse(bb *blackboard.Blackboard, sourceName, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		setBoardEntry(bb, sourceName+":output", raw, sourceName)
		return nil
	}

	if facts, ok := envelope["facts"].([]any); ok {
		for _, item := range facts {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			key, _ := record["key"].(string)
			if strings.TrimSpace(key) == "" {
				continue
			}
			setBoardEntry(bb, key, record["value"], sourceName)
		}
	}

	if artifacts, ok := envelope["artifacts"].([]any); ok {
		for idx, item := range artifacts {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id := firstNonEmpty(stringValue(record["id"]), fmt.Sprintf("%s-artifact-%d", sanitizeName(sourceName), idx+1))
			kind := firstNonEmpty(stringValue(record["kind"]), sanitizeName(sourceName))
			content := encodeBridgeValue(record["content"])
			if err := upsertBoardArtifact(bb, id, kind, content, sourceName); err != nil {
				return err
			}
			if verified, _ := record["verified"].(bool); verified {
				bb.VerifyArtifact(id)
			}
		}
	}

	if issues, ok := envelope["issues"].([]any); ok {
		for idx, item := range issues {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id := firstNonEmpty(stringValue(record["id"]), fmt.Sprintf("%s-issue-%d", sanitizeName(sourceName), idx+1))
			description := firstNonEmpty(stringValue(record["description"]), stringValue(record["summary"]))
			if description == "" {
				continue
			}
			severity := firstNonEmpty(stringValue(record["severity"]), "medium")
			if bb.HasIssue(id) {
				continue
			}
			if err := bb.AddIssue(id, description, severity, sourceName); err != nil {
				return err
			}
		}
	}

	if hypotheses, ok := envelope["hypotheses"].([]any); ok {
		for idx, item := range hypotheses {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id := firstNonEmpty(stringValue(record["id"]), fmt.Sprintf("%s-hypothesis-%d", sanitizeName(sourceName), idx+1))
			description := stringValue(record["description"])
			confidence, _ := record["confidence"].(float64)
			if description == "" {
				continue
			}
			if err := bb.AddHypothesis(id, description, confidence, sourceName); err != nil && !strings.Contains(err.Error(), "already exists") {
				return err
			}
		}
	}

	if summary := stringValue(envelope["summary"]); summary != "" {
		setBoardEntry(bb, sourceName+":summary", summary, sourceName)
	}

	keys := make([]string, 0, len(envelope))
	for key := range envelope {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	setBoardEntry(bb, sourceName+":response_keys", keys, sourceName)
	return nil
}

func upsertBoardArtifact(bb *blackboard.Blackboard, id, kind, content, source string) error {
	for i := range bb.Artifacts {
		if bb.Artifacts[i].ID == id {
			bb.Artifacts[i].Kind = kind
			bb.Artifacts[i].Content = content
			bb.Artifacts[i].Source = source
			return nil
		}
	}
	return bb.AddArtifact(id, kind, content, source)
}

func stringValue(raw any) string {
	if value, ok := raw.(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func sanitizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	replacer := strings.NewReplacer(" ", "_", ":", "_", ".", "_", "/", "_")
	name = replacer.Replace(name)
	if name == "" {
		return "knowledge_source"
	}
	return name
}
