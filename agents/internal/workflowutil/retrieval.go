package workflowutil

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/retrieval"
)

type RetrievalResult struct {
	retrieval.MixedEvidenceResult
}

type RetrievalProvider interface {
	RetrievalService() retrieval.RetrieverService
}

type knowledgeLister interface {
	ListKnowledge(ctx context.Context, workflowID string, kind memory.KnowledgeKind, unresolvedOnly bool) ([]memory.KnowledgeRecord, error)
}

type RetrievalQuery struct {
	Primary       string
	TaskText      string
	StageName     string
	StepID        string
	StepFiles     []string
	Expected      string
	Verification  string
	PreviousNotes []string
}

func Hydrate(ctx context.Context, provider RetrievalProvider, workflowID string, query RetrievalQuery, limit, maxTokens int) (map[string]any, error) {
	if provider == nil || strings.TrimSpace(workflowID) == "" {
		return nil, nil
	}
	service := provider.RetrievalService()
	if service == nil {
		return nil, nil
	}
	queryText := BuildQuery(query)
	if queryText == "" {
		return nil, nil
	}
	blocks, event, err := service.Retrieve(ctx, retrieval.RetrievalQuery{
		Text:      queryText,
		Scope:     "workflow:" + strings.TrimSpace(workflowID),
		MaxTokens: maxTokens,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	results := ContentBlockResults(blocks)
	if lister, ok := provider.(knowledgeLister); ok {
		records, listErr := lister.ListKnowledge(ctx, strings.TrimSpace(workflowID), "", false)
		if listErr != nil {
			return nil, listErr
		}
		results = BuildMixedResults(queryText, blocks, records)
	}
	if len(results) == 0 {
		return nil, nil
	}
	return BuildPayload(queryText, "workflow:"+strings.TrimSpace(workflowID), event, results), nil
}

func ApplyState(state *core.Context, key string, payload map[string]any) {
	if state == nil || strings.TrimSpace(key) == "" || len(payload) == 0 {
		return
	}
	normalizedKey := strings.TrimSpace(key)
	state.Set(normalizedKey, payload)
	state.Set(PayloadKey(normalizedKey), payload)
}

func ApplyTask(task *core.Task, payload map[string]any) *core.Task {
	if task == nil || len(payload) == 0 {
		return task
	}
	cloned := core.CloneTask(task)
	if cloned == nil {
		cloned = &core.Task{}
	}
	if cloned.Context == nil {
		cloned.Context = map[string]any{}
	}
	cloned.Context["workflow_retrieval"] = payload
	cloned.Context["workflow_retrieval_payload"] = payload
	return cloned
}

func PayloadKey(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	return base + "_payload"
}

func StatePayload(state *core.Context, key string) map[string]any {
	if state == nil {
		return nil
	}
	if payload := rawPayload(state.Get(PayloadKey(key))); len(payload) > 0 {
		return payload
	}
	return rawPayload(state.Get(strings.TrimSpace(key)))
}

func TaskPayload(task *core.Task, key string) map[string]any {
	if task == nil || task.Context == nil {
		return nil
	}
	if payload := rawPayloadFromMap(task.Context, PayloadKey(key)); len(payload) > 0 {
		return payload
	}
	return rawPayloadFromMap(task.Context, strings.TrimSpace(key))
}

func RetrievalText(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	if summary := strings.TrimSpace(fmt.Sprint(payload["summary"])); summary != "" && summary != "<nil>" {
		return summary
	}
	// Extract from BuildMixedEvidencePayload results array format.
	if results, ok := payload["results"].([]map[string]any); ok && len(results) > 0 {
		parts := make([]string, 0, len(results))
		for _, result := range results {
			if text := strings.TrimSpace(fmt.Sprint(result["text"])); text != "" && text != "<nil>" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n\n")
		}
	}
	if texts, ok := payload["texts"].([]string); ok && len(texts) > 0 {
		return strings.Join(texts, "\n\n")
	}
	if texts, ok := payload["texts"].([]any); ok && len(texts) > 0 {
		parts := make([]string, 0, len(texts))
		for _, raw := range texts {
			text := strings.TrimSpace(fmt.Sprint(raw))
			if text == "" || text == "<nil>" {
				continue
			}
			parts = append(parts, text)
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n\n")
		}
	}
	return strings.TrimSpace(fmt.Sprint(payload))
}

func ApplyTaskRetrieval(task *core.Task, payload map[string]any) *core.Task {
	if task == nil || len(payload) == 0 {
		return task
	}
	cloned := core.CloneTask(task)
	if cloned == nil {
		cloned = &core.Task{}
	}
	if cloned.Context == nil {
		cloned.Context = map[string]any{}
	}
	cloned.Context["workflow_retrieval"] = RetrievalText(payload)
	cloned.Context["workflow_retrieval_payload"] = payload
	return cloned
}

func ContentBlockResults(blocks []core.ContentBlock) []RetrievalResult {
	source := retrieval.MixedEvidenceResultsFromBlocks(blocks)
	results := make([]RetrievalResult, 0, len(source))
	for _, result := range source {
		results = append(results, RetrievalResult{MixedEvidenceResult: result})
	}
	return results
}

func BuildMixedResults(queryText string, blocks []core.ContentBlock, records []memory.KnowledgeRecord) []RetrievalResult {
	source := retrieval.BuildMixedEvidenceResults(queryText, blocks, records)
	results := make([]RetrievalResult, 0, len(source))
	for _, result := range source {
		results = append(results, RetrievalResult{MixedEvidenceResult: result})
	}
	return results
}

func BuildPayload(queryText, scope string, event retrieval.RetrievalEvent, results []RetrievalResult) map[string]any {
	source := make([]retrieval.MixedEvidenceResult, 0, len(results))
	for _, result := range results {
		source = append(source, result.MixedEvidenceResult)
	}
	return retrieval.BuildMixedEvidencePayload(queryText, scope, event, source)
}

func ParseCitations(raw any) []retrieval.PackedCitation {
	switch typed := raw.(type) {
	case []retrieval.PackedCitation:
		if len(typed) == 0 {
			return nil
		}
		return append([]retrieval.PackedCitation{}, typed...)
	case []any:
		out := make([]retrieval.PackedCitation, 0, len(typed))
		for _, item := range typed {
			entry, ok := item.(retrieval.PackedCitation)
			if ok {
				out = append(out, entry)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

func retrievalResultSummary(result RetrievalResult) string {
	if summary := strings.TrimSpace(result.Summary); summary != "" && summary != "<nil>" {
		return summary
	}
	text := strings.TrimSpace(result.Text)
	if text == "" || text == "<nil>" {
		return ""
	}
	if len(text) > 240 {
		return text[:240] + "..."
	}
	return text
}

func asAnyMap(raw any) map[string]any {
	if raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case map[string]any:
		return typed
	default:
		return nil
	}
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func rawPayload(value any, ok bool) map[string]any {
	if !ok || value == nil {
		return nil
	}
	payload, _ := value.(map[string]any)
	return payload
}

func rawPayloadFromMap(values map[string]any, key string) map[string]any {
	key = strings.TrimSpace(key)
	if key == "" || values == nil {
		return nil
	}
	payload, _ := values[key].(map[string]any)
	return payload
}

func BuildQuery(q RetrievalQuery) string {
	parts := make([]string, 0, 5)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range parts {
			if strings.EqualFold(existing, value) {
				return
			}
		}
		parts = append(parts, value)
	}
	add(q.Primary)
	add(q.TaskText)
	add(q.Expected)
	add(q.Verification)
	for _, note := range q.PreviousNotes {
		add(note)
	}
	return strings.Join(parts, "\n")
}

func TaskPaths(task *core.Task) []string {
	if task == nil {
		return nil
	}
	out := make([]string, 0)
	seen := make(map[string]struct{})
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || value == "<nil>" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, value := range task.Metadata {
		add(value)
	}
	if task.Context != nil {
		for _, key := range []string{"path", "file", "file_path", "target_path", "manifest_path", "database_path"} {
			add(fmt.Sprint(task.Context[key]))
		}
	}
	return out
}
