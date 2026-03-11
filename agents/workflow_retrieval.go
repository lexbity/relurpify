package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/retrieval"
)

type workflowRetrievalResult struct {
	Text      string                    `json:"text"`
	Citations []retrieval.PackedCitation `json:"citations,omitempty"`
}

type workflowRetrievalProvider interface {
	RetrievalService() retrieval.RetrieverService
}

type workflowRetrievalQuery struct {
	Primary       string
	TaskText      string
	StageName     string
	StepID        string
	StepFiles     []string
	Expected      string
	Verification  string
	PreviousNotes []string
}

func hydrateWorkflowRetrieval(ctx context.Context, provider workflowRetrievalProvider, workflowID string, query workflowRetrievalQuery, limit, maxTokens int) (map[string]any, error) {
	if provider == nil || strings.TrimSpace(workflowID) == "" {
		return nil, nil
	}
	service := provider.RetrievalService()
	if service == nil {
		return nil, nil
	}
	queryText := buildWorkflowRetrievalQuery(query)
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
	results := contentBlockResults(blocks)
	if len(results) == 0 {
		return nil, nil
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

func applyWorkflowRetrievalState(state *core.Context, key string, payload map[string]any) {
	if state == nil || strings.TrimSpace(key) == "" || len(payload) == 0 {
		return
	}
	state.Set(strings.TrimSpace(key), payload)
}

func applyWorkflowRetrievalTask(task *core.Task, payload map[string]any) *core.Task {
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

func contentBlockTexts(blocks []core.ContentBlock) []string {
	results := contentBlockResults(blocks)
	out := make([]string, 0, len(results))
	for _, result := range results {
		out = append(out, result.Text)
	}
	return out
}

func contentBlockResults(blocks []core.ContentBlock) []workflowRetrievalResult {
	results := make([]workflowRetrievalResult, 0, len(blocks))
	for _, block := range blocks {
		switch typed := block.(type) {
		case core.TextContentBlock:
			if text := strings.TrimSpace(typed.Text); text != "" {
				results = append(results, workflowRetrievalResult{Text: text})
			}
		case core.StructuredContentBlock:
			payload, ok := typed.Data.(map[string]any)
			if !ok {
				continue
			}
			text := strings.TrimSpace(fmt.Sprint(payload["text"]))
			if text != "" && text != "<nil>" {
				results = append(results, workflowRetrievalResult{
					Text:      text,
					Citations: parseWorkflowRetrievalCitations(payload["citations"]),
				})
			}
		}
	}
	return results
}

func parseWorkflowRetrievalCitations(raw any) []retrieval.PackedCitation {
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

func buildWorkflowRetrievalQuery(q workflowRetrievalQuery) string {
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

func taskRetrievalPaths(task *core.Task) []string {
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
