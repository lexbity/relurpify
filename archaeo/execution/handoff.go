package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

const handoffArtifactKind = "archaeo_execution_handoff"

type HandoffRecorder struct {
	Store memory.WorkflowStateStore
	Now   func() time.Time
	NewID func(prefix string) string
}

func (r HandoffRecorder) Record(ctx context.Context, task *core.Task, state *core.Context, plan *frameworkplan.LivingPlan, step *frameworkplan.PlanStep) (*archaeodomain.ExecutionHandoff, error) {
	store := r.workflowStore()
	if store == nil || plan == nil || strings.TrimSpace(plan.WorkflowID) == "" || plan.Version <= 0 {
		return nil, nil
	}
	now := r.now()
	record := &archaeodomain.ExecutionHandoff{
		ID:                  r.newID("handoff"),
		WorkflowID:          plan.WorkflowID,
		ExplorationID:       stateString(state, "euclo.active_exploration_id"),
		PlanID:              plan.ID,
		PlanVersion:         plan.Version,
		StepID:              stepID(step),
		BasedOnRevision:     firstNonEmpty(stateString(state, "euclo.based_on_revision"), taskContextString(task, "based_on_revision")),
		SemanticSnapshotRef: stateString(state, "euclo.active_exploration_snapshot_id"),
		HandoffAccepted:     true,
		HandoffRef:          fmt.Sprintf("%s:v%d", plan.ID, plan.Version),
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	if err := store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      fmt.Sprintf("archaeo-execution-handoff:%s:%d:%s", plan.ID, plan.Version, record.StepID),
		WorkflowID:      plan.WorkflowID,
		Kind:            handoffArtifactKind,
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     fmt.Sprintf("execution handoff %s", record.HandoffRef),
		SummaryMetadata: map[string]any{"plan_id": plan.ID, "plan_version": plan.Version, "step_id": record.StepID},
		InlineRawText:   string(raw),
		CreatedAt:       now,
	}); err != nil {
		return nil, err
	}
	if err := archaeoevents.AppendWorkflowEvent(ctx, store, plan.WorkflowID, archaeoevents.EventExecutionHandoffRecorded, record.HandoffRef, map[string]any{
		"plan_id":               record.PlanID,
		"plan_version":          record.PlanVersion,
		"exploration_id":        record.ExplorationID,
		"handoff_ref":           record.HandoffRef,
		"step_id":               record.StepID,
		"based_on_revision":     record.BasedOnRevision,
		"semantic_snapshot_ref": record.SemanticSnapshotRef,
	}, now); err != nil {
		return nil, err
	}
	if state != nil {
		state.Set("euclo.execution_handoff", record)
		state.Set("euclo.execution_handoff_ref", record.HandoffRef)
	}
	return record, nil
}

func (r HandoffRecorder) workflowStore() memory.WorkflowStateStore {
	if r.Store == nil {
		return nil
	}
	return r.Store
}

func (r HandoffRecorder) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func (r HandoffRecorder) newID(prefix string) string {
	if r.NewID != nil {
		return r.NewID(prefix)
	}
	return fmt.Sprintf("%s-%d", strings.TrimSpace(prefix), r.now().UnixNano())
}

func stateString(state *core.Context, key string) string {
	if state == nil {
		return ""
	}
	return strings.TrimSpace(state.GetString(key))
}

func taskContextString(task *core.Task, key string) string {
	if task == nil || task.Context == nil {
		return ""
	}
	raw := strings.TrimSpace(fmt.Sprint(task.Context[key]))
	if raw == "<nil>" {
		return ""
	}
	return raw
}

func stepID(step *frameworkplan.PlanStep) string {
	if step == nil {
		return ""
	}
	return strings.TrimSpace(step.ID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
