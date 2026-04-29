package retrieval

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/rex/store"
	"codeburg.org/lexbit/relurpify/named/rex/route"
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

type workflowArtifactLister interface {
	ListWorkflowArtifacts(context.Context, string, string) ([]store.WorkflowArtifactRecord, error)
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
func ExpandWithWorkflowStore(ctx context.Context, workflowStore any, workflowID string, task *core.Task, env *contextdata.Envelope, decision route.RouteDecision) (Expansion, error) {
	provider, ok := workflowStore.(workflowArtifactLister)
	if !ok {
		return Expansion{}, nil
	}
	return expandContext(ctx, provider, workflowID, task, env, ResolvePolicy(decision))
}

// Apply persists expansion into envelope and task context.
func Apply(env *contextdata.Envelope, task *core.Task, expansion Expansion) *core.Task {
	if env != nil {
		env.SetWorkingValue("rex.context_expansion", expansion, contextdata.MemoryClassTask)
		if len(expansion.WorkflowRetrieval) > 0 {
			env.SetWorkingValue("pipeline.workflow_retrieval", expansion.WorkflowRetrieval, contextdata.MemoryClassTask)
		}
	}
	if task == nil {
		return task
	}
	cloned := &core.Task{
		ID:          task.ID,
		Type:        task.Type,
		Instruction: task.Instruction,
	}
	if len(task.Data) > 0 {
		cloned.Data = map[string]any{}
		for k, v := range task.Data {
			cloned.Data[k] = v
		}
	}
	if len(task.Context) > 0 {
		cloned.Context = map[string]any{}
		for k, v := range task.Context {
			cloned.Context[k] = v
		}
	}
	if len(task.Metadata) > 0 {
		cloned.Metadata = map[string]any{}
		for k, v := range task.Metadata {
			cloned.Metadata[k] = v
		}
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

func expandContext(ctx context.Context, provider workflowArtifactLister, workflowID string, task *core.Task, _ *contextdata.Envelope, policy Policy) (Expansion, error) {
	expansion := Expansion{
		LocalPaths:        taskPaths(task),
		ExpansionStrategy: policy.ExpansionStrategy,
	}
	shouldWiden := policy.WidenToWorkflow || (policy.WidenWhenNoLocal && len(expansion.LocalPaths) == 0)
	if !shouldWiden || provider == nil || strings.TrimSpace(workflowID) == "" {
		expansion.Summary = summarizeExpansion(expansion)
		return expansion, nil
	}
	payload, err := hydrateWorkflowRetrieval(ctx, provider, workflowID, policy.WorkflowLimit)
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

func hydrateWorkflowRetrieval(ctx context.Context, provider workflowArtifactLister, workflowID string, limit int) (map[string]any, error) {
	if provider == nil || strings.TrimSpace(workflowID) == "" {
		return nil, nil
	}
	artifacts, err := provider.ListWorkflowArtifacts(ctx, strings.TrimSpace(workflowID), "")
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(artifacts) > limit {
		artifacts = artifacts[:limit]
	}
	serialized := make([]map[string]any, 0, len(artifacts))
	texts := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		entry := map[string]any{
			"artifact_id":    artifact.ArtifactID,
			"kind":           artifact.Kind,
			"content_type":   artifact.ContentType,
			"summary_text":   artifact.SummaryText,
			"storage_kind":   artifact.StorageKind,
			"created_at":     artifact.CreatedAt,
			"run_id":         artifact.RunID,
			"workflow_id":    artifact.WorkflowID,
			"raw_size_bytes":  artifact.RawSizeBytes,
			"compression":    artifact.CompressionMethod,
			"summary_meta":   artifact.SummaryMetadata,
			"inline_raw_text": artifact.InlineRawText,
		}
		if text := strings.TrimSpace(firstNonEmpty(artifact.SummaryText, artifact.InlineRawText)); text != "" {
			entry["text"] = text
			texts = append(texts, text)
		}
		serialized = append(serialized, entry)
	}
	return map[string]any{
		"scope":       "workflow:" + strings.TrimSpace(workflowID),
		"texts":       texts,
		"results":     serialized,
		"summary":     strings.Join(texts, "\n\n"),
		"result_size": len(texts),
	}, nil
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
		add(fmt.Sprint(value))
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
