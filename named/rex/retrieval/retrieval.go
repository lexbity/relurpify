package retrieval

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	frameworkretrieval "github.com/lexcodex/relurpify/framework/retrieval"
	"github.com/lexcodex/relurpify/named/rex/route"
)

type Policy struct {
	ModeID            string `json:"mode_id"`
	ProfileID         string `json:"profile_id"`
	LocalPathsFirst   bool   `json:"local_paths_first"`
	WidenToWorkflow   bool   `json:"widen_to_workflow"`
	WidenWhenNoLocal  bool   `json:"widen_when_no_local"`
	WorkflowLimit     int    `json:"workflow_limit"`
	WorkflowMaxTokens int    `json:"workflow_max_tokens"`
	ExpansionStrategy string `json:"expansion_strategy"`
}

type Expansion struct {
	LocalPaths        []string       `json:"local_paths,omitempty"`
	WorkflowRetrieval map[string]any `json:"workflow_retrieval,omitempty"`
	WidenedToWorkflow bool           `json:"widened_to_workflow"`
	ExpansionStrategy string         `json:"expansion_strategy,omitempty"`
	Summary           string         `json:"summary,omitempty"`
}

type workflowRetrievalProvider interface {
	RetrievalService() frameworkretrieval.RetrieverService
}

type workflowKnowledgeLister interface {
	ListKnowledge(ctx context.Context, workflowID string, kind memory.KnowledgeKind, unresolvedOnly bool) ([]memory.KnowledgeRecord, error)
}

type workflowRetrievalResult struct {
	Text      string                             `json:"text"`
	Citations []frameworkretrieval.PackedCitation `json:"citations,omitempty"`
}

// ResolvePolicy maps rex route decisions to workflow-aware retrieval policy.
func ResolvePolicy(decision route.RouteDecision) Policy {
	policy := Policy{
		ModeID:            decision.Mode,
		ProfileID:         decision.Profile,
		LocalPathsFirst:   true,
		WidenWhenNoLocal:  true,
		WorkflowLimit:     4,
		WorkflowMaxTokens: 500,
		ExpansionStrategy: "local_first",
	}
	switch decision.Family {
	case route.FamilyPlanner:
		policy.WidenToWorkflow = true
		policy.WorkflowLimit = 6
		policy.WorkflowMaxTokens = 800
		policy.ExpansionStrategy = "local_then_workflow"
	case route.FamilyArchitect, route.FamilyPipeline:
		policy.WidenToWorkflow = true
		policy.WorkflowLimit = 5
		policy.WorkflowMaxTokens = 700
		policy.ExpansionStrategy = "local_then_targeted_workflow"
	default:
		policy.WidenToWorkflow = decision.RequireRetrieval
	}
	if decision.RequireRetrieval {
		policy.WidenToWorkflow = true
	}
	return policy
}

// ExpandWithWorkflowStore expands rex context using workflow retrieval surfaces.
func ExpandWithWorkflowStore(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID string, task *core.Task, state *core.Context, decision route.RouteDecision) (Expansion, error) {
	return expandContext(ctx, store, workflowID, task, state, ResolvePolicy(decision))
}

// Apply persists expansion into state and task context.
func Apply(state *core.Context, task *core.Task, expansion Expansion) *core.Task {
	if state != nil {
		state.Set("rex.context_expansion", expansion)
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
		cloned.Context["rex.local_paths"] = append([]string{}, expansion.LocalPaths...)
	}
	cloned.Context["rex.context_expansion"] = expansion
	return cloned
}

func expandContext(ctx context.Context, provider workflowRetrievalProvider, workflowID string, task *core.Task, _ *core.Context, policy Policy) (Expansion, error) {
	expansion := Expansion{
		LocalPaths:        taskPaths(task),
		ExpansionStrategy: policy.ExpansionStrategy,
	}
	shouldWiden := policy.WidenToWorkflow || (policy.WidenWhenNoLocal && len(expansion.LocalPaths) == 0)
	if !shouldWiden || provider == nil || strings.TrimSpace(workflowID) == "" {
		expansion.Summary = summarizeExpansion(expansion)
		return expansion, nil
	}
	payload, err := hydrateWorkflowRetrieval(ctx, provider, workflowID, taskInstruction(task), taskVerification(task), policy.WorkflowLimit, policy.WorkflowMaxTokens)
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

func hydrateWorkflowRetrieval(ctx context.Context, provider workflowRetrievalProvider, workflowID, instruction, verification string, limit, maxTokens int) (map[string]any, error) {
	service := provider.RetrievalService()
	if service == nil || strings.TrimSpace(workflowID) == "" {
		return nil, nil
	}
	queryText := buildWorkflowQuery(instruction, verification)
	if queryText == "" {
		return nil, nil
	}
	blocks, event, err := service.Retrieve(ctx, frameworkretrieval.RetrievalQuery{
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
					results = append(results, workflowRetrievalResult{Text: text})
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

func buildWorkflowQuery(instruction, verification string) string {
	parts := make([]string, 0, 2)
	for _, value := range []string{instruction, verification} {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, "\n")
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
					Citations: parseCitations(payload["citations"]),
				})
			}
		}
	}
	return results
}

func parseCitations(raw any) []frameworkretrieval.PackedCitation {
	switch typed := raw.(type) {
	case []frameworkretrieval.PackedCitation:
		return append([]frameworkretrieval.PackedCitation{}, typed...)
	case []any:
		out := make([]frameworkretrieval.PackedCitation, 0, len(typed))
		for _, item := range typed {
			entry, ok := item.(frameworkretrieval.PackedCitation)
			if ok {
				out = append(out, entry)
			}
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

func summarizeExpansion(expansion Expansion) string {
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
