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
	Text      string                     `json:"text"`
	Citations []retrieval.PackedCitation `json:"citations,omitempty"`
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
	if len(results) == 0 {
		if lister, ok := provider.(knowledgeLister); ok {
			records, listErr := lister.ListKnowledge(ctx, strings.TrimSpace(workflowID), "", false)
			if listErr != nil {
				return nil, listErr
			}
			for _, rec := range records {
				parts := make([]string, 0, 2)
				if t := strings.TrimSpace(rec.Title); t != "" {
					parts = append(parts, t)
				}
				if c := strings.TrimSpace(rec.Content); c != "" {
					parts = append(parts, c)
				}
				text := strings.Join(parts, ": ")
				if text != "" {
					results = append(results, RetrievalResult{Text: text})
				}
			}
		}
		if len(results) == 0 {
			return nil, nil
		}
	}
	texts := make([]string, 0, len(results))
	citationCount := 0
	serializedResults := make([]map[string]any, 0, len(results))
	for _, result := range results {
		texts = append(texts, result.Text)
		citationCount += len(result.Citations)
		entry := map[string]any{"text": result.Text}
		if len(result.Citations) > 0 {
			entry["citations"] = result.Citations
		}
		serializedResults = append(serializedResults, entry)
	}
	return map[string]any{
		"query":          queryText,
		"scope":          "workflow:" + strings.TrimSpace(workflowID),
		"cache_tier":     event.CacheTier,
		"query_id":       event.QueryID,
		"texts":          texts,
		"results":        serializedResults,
		"summary":        strings.Join(texts, "\n\n"),
		"result_size":    len(texts),
		"citation_count": citationCount,
	}, nil
}

func ApplyState(state *core.Context, key string, payload map[string]any) {
	if state == nil || strings.TrimSpace(key) == "" || len(payload) == 0 {
		return
	}
	state.Set(strings.TrimSpace(key), payload)
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
	return cloned
}

func RetrievalText(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	if summary := strings.TrimSpace(fmt.Sprint(payload["summary"])); summary != "" && summary != "<nil>" {
		return summary
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
	results := make([]RetrievalResult, 0, len(blocks))
	for _, block := range blocks {
		switch typed := block.(type) {
		case core.TextContentBlock:
			if text := strings.TrimSpace(typed.Text); text != "" {
				results = append(results, RetrievalResult{Text: text})
			}
		case core.StructuredContentBlock:
			payload, ok := typed.Data.(map[string]any)
			if !ok {
				continue
			}
			text := strings.TrimSpace(fmt.Sprint(payload["text"]))
			if text != "" && text != "<nil>" {
				results = append(results, RetrievalResult{
					Text:      text,
					Citations: ParseCitations(payload["citations"]),
				})
			}
		}
	}
	return results
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
