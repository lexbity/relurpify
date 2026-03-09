package persistence

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
)

type WorkflowProjectionTier string

const (
	WorkflowProjectionTierHot  WorkflowProjectionTier = "hot"
	WorkflowProjectionTierWarm WorkflowProjectionTier = "warm"
	WorkflowProjectionTierCold WorkflowProjectionTier = "cold"
)

type WorkflowProjectionRole = core.CoordinationRole

type WorkflowResourceRef struct {
	WorkflowID string
	RunID      string
	StepID     string
	Tier       WorkflowProjectionTier
	Role       WorkflowProjectionRole
}

type WorkflowProjectionService struct {
	Store WorkflowStateStore
}

type WorkflowResourceCapability struct {
	service WorkflowProjectionService
	ref     WorkflowResourceRef
}

func DefaultWorkflowProjectionRefs(workflowID, runID, stepID string, role WorkflowProjectionRole) []string {
	tiers := defaultProjectionTiersForRole(role)
	refs := make([]string, 0, len(tiers))
	for _, tier := range tiers {
		ref := WorkflowResourceRef{
			WorkflowID: strings.TrimSpace(workflowID),
			RunID:      strings.TrimSpace(runID),
			StepID:     strings.TrimSpace(stepID),
			Tier:       tier,
			Role:       role,
		}
		if tier == WorkflowProjectionTierCold {
			ref.StepID = ""
		}
		if err := ref.Validate(); err != nil {
			continue
		}
		refs = append(refs, BuildWorkflowResourceURI(ref))
	}
	return refs
}

func BuildWorkflowResourceURI(ref WorkflowResourceRef) string {
	values := url.Values{}
	if strings.TrimSpace(ref.RunID) != "" {
		values.Set("run", strings.TrimSpace(ref.RunID))
	}
	if strings.TrimSpace(ref.StepID) != "" {
		values.Set("step", strings.TrimSpace(ref.StepID))
	}
	if strings.TrimSpace(string(ref.Role)) != "" {
		values.Set("role", strings.TrimSpace(string(ref.Role)))
	}
	return (&url.URL{
		Scheme:   "workflow",
		Host:     strings.TrimSpace(ref.WorkflowID),
		Path:     "/" + strings.TrimSpace(string(ref.Tier)),
		RawQuery: values.Encode(),
	}).String()
}

func defaultProjectionTiersForRole(role WorkflowProjectionRole) []WorkflowProjectionTier {
	switch role {
	case core.CoordinationRolePlanner:
		return []WorkflowProjectionTier{WorkflowProjectionTierHot, WorkflowProjectionTierWarm}
	case core.CoordinationRoleArchitect:
		return []WorkflowProjectionTier{WorkflowProjectionTierHot, WorkflowProjectionTierWarm}
	case core.CoordinationRoleReviewer, core.CoordinationRoleVerifier:
		return []WorkflowProjectionTier{WorkflowProjectionTierWarm, WorkflowProjectionTierCold}
	case core.CoordinationRoleExecutor:
		return []WorkflowProjectionTier{WorkflowProjectionTierHot}
	case core.CoordinationRoleBackgroundAgent:
		return []WorkflowProjectionTier{WorkflowProjectionTierWarm, WorkflowProjectionTierCold}
	default:
		return []WorkflowProjectionTier{WorkflowProjectionTierWarm}
	}
}

func ParseWorkflowResourceURI(raw string) (WorkflowResourceRef, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return WorkflowResourceRef{}, err
	}
	ref := WorkflowResourceRef{
		WorkflowID: strings.TrimSpace(parsed.Host),
		RunID:      strings.TrimSpace(parsed.Query().Get("run")),
		StepID:     strings.TrimSpace(parsed.Query().Get("step")),
		Tier:       WorkflowProjectionTier(strings.TrimPrefix(strings.TrimSpace(parsed.Path), "/")),
		Role:       WorkflowProjectionRole(strings.TrimSpace(parsed.Query().Get("role"))),
	}
	if parsed.Scheme != "workflow" {
		return WorkflowResourceRef{}, fmt.Errorf("workflow resource uri must use workflow:// scheme")
	}
	if err := ref.Validate(); err != nil {
		return WorkflowResourceRef{}, err
	}
	return ref, nil
}

func (r WorkflowResourceRef) Validate() error {
	if strings.TrimSpace(r.WorkflowID) == "" {
		return fmt.Errorf("workflow id required")
	}
	switch r.Tier {
	case WorkflowProjectionTierHot, WorkflowProjectionTierWarm, WorkflowProjectionTierCold:
	default:
		return fmt.Errorf("projection tier %q invalid", r.Tier)
	}
	switch r.Role {
	case "",
		core.CoordinationRolePlanner,
		core.CoordinationRoleArchitect,
		core.CoordinationRoleReviewer,
		core.CoordinationRoleVerifier,
		core.CoordinationRoleExecutor,
		core.CoordinationRoleDomainPack,
		core.CoordinationRoleBackgroundAgent:
	default:
		return fmt.Errorf("projection role %q invalid", r.Role)
	}
	return nil
}

func NewWorkflowResourceCapability(store WorkflowStateStore, ref WorkflowResourceRef) (*WorkflowResourceCapability, error) {
	if store == nil {
		return nil, fmt.Errorf("workflow state store required")
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	return &WorkflowResourceCapability{
		service: WorkflowProjectionService{Store: store},
		ref:     ref,
	}, nil
}

func (s WorkflowProjectionService) Project(ctx context.Context, ref WorkflowResourceRef) (*core.ResourceReadResult, error) {
	if s.Store == nil {
		return nil, fmt.Errorf("workflow projection store unavailable")
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	payload, err := s.projectPayload(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &core.ResourceReadResult{
		Contents: []core.ContentBlock{
			core.StructuredContentBlock{Data: payload},
		},
		Metadata: map[string]any{
			"workflow_uri": BuildWorkflowResourceURI(ref),
			"workflow_id":  ref.WorkflowID,
			"run_id":       ref.RunID,
			"step_id":      ref.StepID,
			"tier":         string(ref.Tier),
			"role":         string(ref.Role),
		},
	}, nil
}

func (c *WorkflowResourceCapability) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	uri := BuildWorkflowResourceURI(c.ref)
	nameParts := []string{"workflow", c.ref.WorkflowID, string(c.ref.Tier)}
	if c.ref.StepID != "" {
		nameParts = append(nameParts, c.ref.StepID)
	}
	if c.ref.Role != "" {
		nameParts = append(nameParts, string(c.ref.Role))
	}
	name := strings.Join(nameParts, ".")
	return core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
		ID:          "resource:" + sanitizeWorkflowResourceName(name),
		Kind:        core.CapabilityKindResource,
		Name:        sanitizeWorkflowResourceName(name),
		Description: fmt.Sprintf("%s workflow projection resource", string(c.ref.Tier)),
		Category:    "workflow-projection",
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeWorkspace,
		},
		TrustClass: core.TrustClassWorkspaceTrusted,
		Availability: core.AvailabilitySpec{
			Available: true,
		},
		Annotations: map[string]any{
			"workflow_uri":     uri,
			"workflow_id":      c.ref.WorkflowID,
			"workflow_run_id":  c.ref.RunID,
			"workflow_step_id": c.ref.StepID,
			"projection_tier":  string(c.ref.Tier),
			"projection_role":  string(c.ref.Role),
		},
	})
}

func (c *WorkflowResourceCapability) ReadResource(ctx context.Context, _ *core.Context) (*core.ResourceReadResult, error) {
	return c.service.Project(ctx, c.ref)
}

func (s WorkflowProjectionService) projectPayload(ctx context.Context, ref WorkflowResourceRef) (map[string]any, error) {
	workflow, ok, err := s.Store.GetWorkflow(ctx, ref.WorkflowID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("workflow %s not found", ref.WorkflowID)
	}
	payload := map[string]any{
		"uri":         BuildWorkflowResourceURI(ref),
		"workflow_id": workflow.WorkflowID,
		"task_id":     workflow.TaskID,
		"task_type":   workflow.TaskType,
		"instruction": workflow.Instruction,
		"status":      workflow.Status,
		"tier":        string(ref.Tier),
		"role":        string(ref.Role),
	}
	if strings.TrimSpace(ref.RunID) != "" {
		run, ok, err := s.Store.GetRun(ctx, ref.RunID)
		if err != nil {
			return nil, err
		}
		if ok {
			payload["run"] = map[string]any{
				"run_id":      run.RunID,
				"status":      run.Status,
				"agent_name":  run.AgentName,
				"agent_mode":  run.AgentMode,
				"started_at":  run.StartedAt,
				"finished_at": run.FinishedAt,
			}
		}
	}
	switch ref.Tier {
	case WorkflowProjectionTierHot:
		return s.projectHot(ctx, payload, ref)
	case WorkflowProjectionTierWarm:
		return s.projectWarm(ctx, payload, ref)
	case WorkflowProjectionTierCold:
		return s.projectCold(ctx, payload, ref)
	default:
		return nil, fmt.Errorf("projection tier %q invalid", ref.Tier)
	}
}

func (s WorkflowProjectionService) projectHot(ctx context.Context, payload map[string]any, ref WorkflowResourceRef) (map[string]any, error) {
	if ref.StepID != "" {
		slice, ok, err := s.Store.LoadStepSlice(ctx, ref.WorkflowID, ref.StepID, 10)
		if err != nil {
			return nil, err
		}
		if ok {
			payload["step"] = hotStepProjection(slice, ref.Role)
		}
	}
	if ref.Role == core.CoordinationRolePlanner || ref.StepID == "" {
		payload["plan"] = workflowPlanProjection(ctx, s.Store, ref.WorkflowID)
	}
	return payload, nil
}

func (s WorkflowProjectionService) projectWarm(ctx context.Context, payload map[string]any, ref WorkflowResourceRef) (map[string]any, error) {
	payload, err := s.projectHot(ctx, payload, ref)
	if err != nil {
		return nil, err
	}
	steps, err := s.Store.ListSteps(ctx, ref.WorkflowID)
	if err != nil {
		return nil, err
	}
	payload["steps"] = workflowStepsProjection(steps)
	artifacts, err := s.Store.ListWorkflowArtifacts(ctx, ref.WorkflowID, ref.RunID)
	if err != nil {
		return nil, err
	}
	payload["workflow_artifacts"] = workflowArtifactSummaries(artifacts, false)
	issues, err := s.Store.ListKnowledge(ctx, ref.WorkflowID, KnowledgeKindIssue, true)
	if err != nil {
		return nil, err
	}
	decisions, err := s.Store.ListKnowledge(ctx, ref.WorkflowID, KnowledgeKindDecision, false)
	if err != nil {
		return nil, err
	}
	payload["issues"] = knowledgeProjection(issues, false)
	payload["decisions"] = knowledgeProjection(decisions, false)
	return payload, nil
}

func (s WorkflowProjectionService) projectCold(ctx context.Context, payload map[string]any, ref WorkflowResourceRef) (map[string]any, error) {
	payload, err := s.projectWarm(ctx, payload, ref)
	if err != nil {
		return nil, err
	}
	artifacts, err := s.Store.ListWorkflowArtifacts(ctx, ref.WorkflowID, ref.RunID)
	if err != nil {
		return nil, err
	}
	payload["workflow_artifacts"] = workflowArtifactSummaries(artifacts, true)
	if ref.RunID != "" {
		stageResults, err := s.Store.ListStageResults(ctx, ref.WorkflowID, ref.RunID)
		if err != nil {
			return nil, err
		}
		payload["stage_results"] = stageResultProjection(stageResults)
		providers, err := s.Store.ListProviderSnapshots(ctx, ref.WorkflowID, ref.RunID)
		if err != nil {
			return nil, err
		}
		payload["provider_snapshots"] = providerSnapshotProjection(providers)
		sessions, err := s.Store.ListProviderSessionSnapshots(ctx, ref.WorkflowID, ref.RunID)
		if err != nil {
			return nil, err
		}
		payload["provider_sessions"] = providerSessionProjection(sessions)
	}
	events, err := s.Store.ListEvents(ctx, ref.WorkflowID, 25)
	if err != nil {
		return nil, err
	}
	payload["events"] = eventProjection(events)
	return payload, nil
}

func hotStepProjection(slice *WorkflowStepSlice, role WorkflowProjectionRole) map[string]any {
	if slice == nil {
		return nil
	}
	payload := map[string]any{
		"step_id":      slice.Step.StepID,
		"description":  slice.Step.Step.Description,
		"status":       slice.Step.Status,
		"summary":      slice.Step.Summary,
		"dependencies": dependencyRunSummaries(slice.DependencyRuns),
	}
	if role != core.CoordinationRolePlanner {
		payload["facts"] = knowledgeProjection(slice.Facts, false)
		payload["issues"] = knowledgeProjection(slice.Issues, false)
		payload["decisions"] = knowledgeProjection(slice.Decisions, false)
		payload["artifacts"] = stepArtifactProjection(slice.Artifacts, false)
	}
	return payload
}

func workflowPlanProjection(ctx context.Context, store WorkflowStateStore, workflowID string) map[string]any {
	plan, ok, err := store.GetActivePlan(ctx, workflowID)
	if err != nil || !ok {
		return nil
	}
	steps := make([]map[string]any, 0, len(plan.Plan.Steps))
	for _, step := range plan.Plan.Steps {
		steps = append(steps, map[string]any{
			"id":           step.ID,
			"description":  step.Description,
			"expected":     step.Expected,
			"verification": step.Verification,
			"files":        append([]string{}, step.Files...),
		})
	}
	return map[string]any{
		"plan_id": plan.PlanID,
		"goal":    plan.Plan.Goal,
		"steps":   steps,
	}
}

func workflowStepsProjection(steps []WorkflowStepRecord) []map[string]any {
	out := make([]map[string]any, 0, len(steps))
	for _, step := range steps {
		out = append(out, map[string]any{
			"step_id":    step.StepID,
			"ordinal":    step.Ordinal,
			"status":     step.Status,
			"summary":    step.Summary,
			"depends_on": append([]string{}, step.Dependencies...),
		})
	}
	return out
}

func dependencyRunSummaries(runs []StepRunRecord) []map[string]any {
	out := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		out = append(out, map[string]any{
			"step_id":         run.StepID,
			"attempt":         run.Attempt,
			"status":          run.Status,
			"summary":         run.Summary,
			"verification_ok": run.VerificationOK,
		})
	}
	return out
}

func knowledgeProjection(records []KnowledgeRecord, includeMetadata bool) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		item := map[string]any{
			"record_id": record.RecordID,
			"step_id":   record.StepID,
			"title":     record.Title,
			"content":   record.Content,
			"status":    record.Status,
		}
		if includeMetadata && len(record.Metadata) > 0 {
			item["metadata"] = cloneAnyMap(record.Metadata)
		}
		out = append(out, item)
	}
	return out
}

func stepArtifactProjection(records []StepArtifactRecord, includeRaw bool) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		item := map[string]any{
			"artifact_id":        record.ArtifactID,
			"step_run_id":        record.StepRunID,
			"kind":               record.Kind,
			"content_type":       record.ContentType,
			"summary_text":       record.SummaryText,
			"summary_metadata":   cloneAnyMap(record.SummaryMetadata),
			"storage_kind":       record.StorageKind,
			"compression_method": record.CompressionMethod,
		}
		if includeRaw {
			item["inline_raw_text"] = record.InlineRawText
			item["raw_ref"] = record.RawRef
			item["raw_size_bytes"] = record.RawSizeBytes
		}
		out = append(out, item)
	}
	return out
}

func workflowArtifactSummaries(records []WorkflowArtifactRecord, includeRaw bool) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		item := map[string]any{
			"artifact_id":        record.ArtifactID,
			"run_id":             record.RunID,
			"kind":               record.Kind,
			"content_type":       record.ContentType,
			"summary_text":       record.SummaryText,
			"summary_metadata":   cloneAnyMap(record.SummaryMetadata),
			"storage_kind":       record.StorageKind,
			"compression_method": record.CompressionMethod,
		}
		if includeRaw {
			item["inline_raw_text"] = record.InlineRawText
			item["raw_ref"] = record.RawRef
			item["raw_size_bytes"] = record.RawSizeBytes
		}
		out = append(out, item)
	}
	return out
}

func stageResultProjection(records []WorkflowStageResultRecord) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, map[string]any{
			"result_id":         record.ResultID,
			"stage_name":        record.StageName,
			"stage_index":       record.StageIndex,
			"contract_name":     record.ContractName,
			"contract_version":  record.ContractVersion,
			"validation_ok":     record.ValidationOK,
			"error_text":        record.ErrorText,
			"retry_attempt":     record.RetryAttempt,
			"transition_kind":   record.TransitionKind,
			"transition_reason": record.TransitionReason,
			"decoded_output":    record.DecodedOutput,
		})
	}
	return out
}

func providerSnapshotProjection(records []WorkflowProviderSnapshotRecord) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, map[string]any{
			"snapshot_id":    record.SnapshotID,
			"provider_id":    record.ProviderID,
			"recoverability": record.Recoverability,
			"capability_ids": append([]string{}, record.CapabilityIDs...),
			"health":         record.Health.Status,
			"metadata":       cloneAnyMap(record.Metadata),
		})
	}
	return out
}

func providerSessionProjection(records []WorkflowProviderSessionSnapshotRecord) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, map[string]any{
			"snapshot_id": record.SnapshotID,
			"session_id":  record.Session.ID,
			"provider_id": record.Session.ProviderID,
			"health":      record.Session.Health,
			"metadata":    cloneAnyMap(record.Metadata),
			"state":       record.State,
		})
	}
	return out
}

func eventProjection(records []WorkflowEventRecord) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, map[string]any{
			"event_id":   record.EventID,
			"run_id":     record.RunID,
			"step_id":    record.StepID,
			"event_type": record.EventType,
			"message":    record.Message,
			"metadata":   cloneAnyMap(record.Metadata),
			"created_at": record.CreatedAt,
		})
	}
	return out
}

func sanitizeWorkflowResourceName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	replacer := strings.NewReplacer(":", ".", "/", ".", "?", ".", "&", ".", "=", ".", " ", ".")
	value = replacer.Replace(value)
	parts := strings.Split(value, ".")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, ".")
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out[key] = input[key]
	}
	return out
}
