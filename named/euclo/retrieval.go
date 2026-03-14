package euclo

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/retrieval"
)

type RetrievalPolicy struct {
	ModeID            string `json:"mode_id"`
	ProfileID         string `json:"profile_id"`
	LocalPathsFirst   bool   `json:"local_paths_first"`
	WidenToWorkflow   bool   `json:"widen_to_workflow"`
	WidenWhenNoLocal  bool   `json:"widen_when_no_local"`
	WorkflowLimit     int    `json:"workflow_limit"`
	WorkflowMaxTokens int    `json:"workflow_max_tokens"`
	ExpansionStrategy string `json:"expansion_strategy"`
}

type ContextExpansion struct {
	LocalPaths        []string       `json:"local_paths,omitempty"`
	WorkflowRetrieval map[string]any `json:"workflow_retrieval,omitempty"`
	WidenedToWorkflow bool           `json:"widened_to_workflow"`
	ExpansionStrategy string         `json:"expansion_strategy,omitempty"`
	Summary           string         `json:"summary,omitempty"`
}

type workflowRetrievalProvider interface {
	RetrievalService() retrieval.RetrieverService
}

type workflowKnowledgeLister interface {
	ListKnowledge(ctx context.Context, workflowID string, kind memory.KnowledgeKind, unresolvedOnly bool) ([]memory.KnowledgeRecord, error)
}

func ResolveRetrievalPolicy(mode ModeResolution, profile ExecutionProfileSelection) RetrievalPolicy {
	policy := RetrievalPolicy{
		ModeID:            mode.ModeID,
		ProfileID:         profile.ProfileID,
		LocalPathsFirst:   true,
		WidenWhenNoLocal:  true,
		WorkflowLimit:     4,
		WorkflowMaxTokens: 500,
		ExpansionStrategy: "local_first",
	}
	switch mode.ModeID {
	case "planning", "review":
		policy.WidenToWorkflow = true
		policy.WorkflowLimit = 6
		policy.WorkflowMaxTokens = 800
		policy.ExpansionStrategy = "local_then_workflow"
	case "debug":
		policy.WidenToWorkflow = true
		policy.WorkflowLimit = 5
		policy.WorkflowMaxTokens = 700
		policy.ExpansionStrategy = "local_then_targeted_workflow"
	default:
		policy.WidenToWorkflow = false
	}
	return policy
}

func ExpandContext(ctx context.Context, provider workflowRetrievalProvider, workflowID string, task *core.Task, state *core.Context, policy RetrievalPolicy) (ContextExpansion, error) {
	expansion := ContextExpansion{
		LocalPaths:        taskPaths(task),
		ExpansionStrategy: policy.ExpansionStrategy,
	}
	shouldWiden := policy.WidenToWorkflow || (policy.WidenWhenNoLocal && len(expansion.LocalPaths) == 0)
	if !shouldWiden || provider == nil || strings.TrimSpace(workflowID) == "" {
		expansion.Summary = summarizeExpansion(expansion)
		return expansion, nil
	}
	payload, err := hydrateWorkflowRetrieval(ctx, provider, workflowID, workflowRetrievalQuery{
		Primary:      taskInstruction(task),
		TaskText:     taskInstruction(task),
		Verification: taskVerification(task),
	}, policy.WorkflowLimit, policy.WorkflowMaxTokens)
	if err != nil {
		return expansion, err
	}
	if len(payload) > 0 {
		expansion.WorkflowRetrieval = payload
		expansion.WidenedToWorkflow = true
	}
	expansion.Summary = summarizeExpansion(expansion)
	return expansion, nil
}

func applyContextExpansion(state *core.Context, task *core.Task, expansion ContextExpansion) *core.Task {
	if state != nil {
		state.Set("euclo.context_expansion", expansion)
		if len(expansion.WorkflowRetrieval) > 0 {
			state.Set("pipeline.workflow_retrieval", expansion.WorkflowRetrieval)
		}
	}
	if task == nil {
		return task
	}
	cloned := core.CloneTask(task)
	if cloned == nil {
		cloned = &core.Task{}
	}
	if cloned.Context == nil {
		cloned.Context = map[string]any{}
	}
	if len(expansion.WorkflowRetrieval) > 0 {
		cloned.Context["workflow_retrieval"] = expansion.WorkflowRetrieval
	}
	if len(expansion.LocalPaths) > 0 {
		cloned.Context["euclo.local_paths"] = append([]string{}, expansion.LocalPaths...)
	}
	cloned.Context["euclo.context_expansion"] = expansion
	return cloned
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
		if lister, ok := provider.(workflowKnowledgeLister); ok {
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
				if text := strings.Join(parts, ": "); text != "" {
					results = append(results, WorkflowRetrievalResult{Text: text})
				}
			}
		}
		if len(results) == 0 {
			return nil, nil
		}
	}
	texts := make([]string, 0, len(results))
	citationCount := 0
	serialized := make([]map[string]any, 0, len(results))
	for _, result := range results {
		texts = append(texts, result.Text)
		citationCount += len(result.Citations)
		entry := map[string]any{"text": result.Text}
		if len(result.Citations) > 0 {
			entry["citations"] = result.Citations
		}
		serialized = append(serialized, entry)
	}
	return map[string]any{
		"query":          queryText,
		"scope":          "workflow:" + strings.TrimSpace(workflowID),
		"cache_tier":     event.CacheTier,
		"query_id":       event.QueryID,
		"texts":          texts,
		"results":        serialized,
		"summary":        strings.Join(texts, "\n\n"),
		"result_size":    len(texts),
		"citation_count": citationCount,
	}, nil
}

type WorkflowRetrievalResult struct {
	Text      string                     `json:"text"`
	Citations []retrieval.PackedCitation `json:"citations,omitempty"`
}

type workflowRetrievalQuery struct {
	Primary       string
	TaskText      string
	Expected      string
	Verification  string
	PreviousNotes []string
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

func contentBlockResults(blocks []core.ContentBlock) []WorkflowRetrievalResult {
	results := make([]WorkflowRetrievalResult, 0, len(blocks))
	for _, block := range blocks {
		switch typed := block.(type) {
		case core.TextContentBlock:
			if text := strings.TrimSpace(typed.Text); text != "" {
				results = append(results, WorkflowRetrievalResult{Text: text})
			}
		case core.StructuredContentBlock:
			payload, ok := typed.Data.(map[string]any)
			if !ok {
				continue
			}
			text := strings.TrimSpace(fmt.Sprint(payload["text"]))
			if text != "" && text != "<nil>" {
				results = append(results, WorkflowRetrievalResult{
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

func taskPaths(task *core.Task) []string {
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

func summarizeExpansion(expansion ContextExpansion) string {
	parts := make([]string, 0, 2)
	if len(expansion.LocalPaths) > 0 {
		parts = append(parts, fmt.Sprintf("local_paths=%d", len(expansion.LocalPaths)))
	}
	if expansion.WidenedToWorkflow && len(expansion.WorkflowRetrieval) > 0 {
		parts = append(parts, "workflow_retrieval")
	}
	return strings.Join(parts, " ")
}

func taskInstruction(task *core.Task) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.Instruction)
}

func taskVerification(task *core.Task) string {
	if task == nil || task.Context == nil {
		return ""
	}
	value := strings.TrimSpace(fmt.Sprint(task.Context["verification"]))
	if value == "<nil>" {
		return ""
	}
	return value
}
