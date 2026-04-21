package session

import (
	"context"
	"fmt"
	"time"

	archaeodomain "codeburg.org/lexbit/relurpify/archaeo/domain"
	archaeoplans "codeburg.org/lexbit/relurpify/archaeo/plans"
	"codeburg.org/lexbit/relurpify/framework/memory"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

// SessionResumeContext is the fully resolved session restoration context.
// It is assembled deterministically from the workflow store and plan store
// with no LLM involvement, then injected into the execution lifecycle
// before initializeManagedExecution runs.
type SessionResumeContext struct {
	// WorkflowID is the workflow being resumed.
	WorkflowID string

	// RunID will be a new run ID for this resume, preserving workflow
	// continuity without clobbering prior run records.
	RunID string

	// Mode is the resolved mode for this session.
	Mode string

	// RootChunkIDs are the BKC root chunk IDs anchored to the active
	// plan version. Used to seed wrapBKCStrategy.
	RootChunkIDs []string

	// ActivePlanVersion is the version to restore.
	ActivePlanVersion int

	// ActivePlanSummary is the resolved text summary of the plan.
	ActivePlanSummary string

	// PhaseState is the persisted archaeology phase state, if present.
	PhaseState *archaeodomain.WorkflowPhaseState

	// SemanticSummary carries resolved pattern, tension, and learning
	// interaction summaries for ExecutorSemanticContext population.
	SemanticSummary SessionSemanticSummary

	// CodeRevision is the git SHA at which the session's BKC context
	// was last anchored.
	CodeRevision string

	// SessionStartTime is when the workflow session was originally created.
	SessionStartTime time.Time
}

// SessionSemanticSummary is the archaeology-domain pre-resolved content
// for this session. Populated from archaeo services during resolution.
type SessionSemanticSummary struct {
	Patterns             []SemanticFindingSummary
	Tensions             []SemanticFindingSummary
	LearningInteractions []SemanticFindingSummary
}

// SemanticFindingSummary is a minimal summary of a semantic finding for
// executor context population.
type SemanticFindingSummary struct {
	ID      string
	Title   string
	Summary string
	Kind    string
	Status  string
}

// IsEmpty returns true when no semantic restoration is possible for
// this session (no BKC context, no plan, no phase state).
func (s SessionResumeContext) IsEmpty() bool {
	return s.WorkflowID == "" ||
		(len(s.RootChunkIDs) == 0 &&
			s.ActivePlanVersion == 0 &&
			s.PhaseState == nil)
}

// IsEmpty returns true when the semantic summary has no content.
func (s SessionSemanticSummary) IsEmpty() bool {
	return len(s.Patterns) == 0 && len(s.Tensions) == 0 && len(s.LearningInteractions) == 0
}

// ToExecutorSemanticContext converts this semantic summary to an ExecutorSemanticContext.
// The activePlanSummary is passed through to the result.
func (s SessionSemanticSummary) ToExecutorSemanticContext(activePlanSummary string) euclotypes.ExecutorSemanticContext {
	return euclotypes.ExecutorSemanticContext{
		Patterns:             convertFindingSummaries(s.Patterns),
		Tensions:             convertFindingSummaries(s.Tensions),
		LearningInteractions: convertFindingSummaries(s.LearningInteractions),
		ActivePlanSummary:    activePlanSummary,
	}
}

func convertFindingSummaries(summaries []SemanticFindingSummary) []euclotypes.SemanticFindingSummary {
	out := make([]euclotypes.SemanticFindingSummary, len(summaries))
	for i, s := range summaries {
		out[i] = euclotypes.SemanticFindingSummary{
			ID:      s.ID,
			Title:   s.Title,
			Summary: s.Summary,
			Kind:    s.Kind,
			Status:  s.Status,
		}
	}
	return out
}

// SessionResumeResolver resolves a full SessionResumeContext from a
// workflow ID. All resolution is deterministic — no LLM calls.
type SessionResumeResolver struct {
	WorkflowStore memory.WorkflowStateStore
	PlanStore     frameworkplan.PlanStore
}

// Resolve returns a fully populated SessionResumeContext for the given
// workflow ID. Returns an error if the workflow does not exist.
func (r *SessionResumeResolver) Resolve(ctx context.Context, workflowID string) (SessionResumeContext, error) {
	// 1. Verify workflow exists.
	wf, ok, err := r.WorkflowStore.GetWorkflow(ctx, workflowID)
	if err != nil {
		return SessionResumeContext{}, fmt.Errorf("session resume: workflow lookup: %w", err)
	}
	if !ok {
		return SessionResumeContext{}, fmt.Errorf("session resume: workflow %q not found", workflowID)
	}

	resume := SessionResumeContext{
		WorkflowID:       workflowID,
		RunID:            fmt.Sprintf("%s-resume-%d", workflowID, time.Now().UnixNano()),
		Mode:             "",
		SessionStartTime: wf.CreatedAt,
	}

	// Try to get mode from workflow metadata (best-effort)
	if mode, ok := wf.Metadata["mode"].(string); ok {
		resume.Mode = mode
	}

	// 3. Resolve active living plan and BKC root chunk IDs.
	planSvc := archaeoplans.Service{Store: r.PlanStore, WorkflowStore: r.WorkflowStore}
	if plan, err := planSvc.LoadActiveVersion(ctx, workflowID); err == nil && plan != nil {
		resume.ActivePlanVersion = plan.Version
		resume.ActivePlanSummary = plan.Plan.Title
		resume.RootChunkIDs = append([]string(nil), plan.RootChunkIDs...)
		resume.CodeRevision = plan.BasedOnRevision
	}

	// 4. Phase state resolution is deferred - the WorkflowStateStore interface
	// does not provide GetWorkflowState. Phase state will be resolved during
	// lifecycle injection from available sources.
	_ = archaeodomain.WorkflowPhaseState{} // reference for future implementation

	return resume, nil
}

// ResolveWithServices returns a fully populated SessionResumeContext using
// additional archaeo services for semantic summary resolution.
func (r *SessionResumeResolver) ResolveWithServices(ctx context.Context, workflowID string, services SemanticResolutionServices) (SessionResumeContext, error) {
	resume, err := r.Resolve(ctx, workflowID)
	if err != nil {
		return resume, err
	}

	// 5. Resolve semantic summary (patterns, tensions, learning interactions).
	// Failures are non-fatal — partial summary is acceptable.
	resume.SemanticSummary = r.resolveSemanticSummary(ctx, workflowID, services)

	return resume, nil
}

// SemanticResolutionServices provides optional services for resolving
// semantic content during session resume.
type SemanticResolutionServices struct {
	// Tensions provides access to tension records for the workflow.
	Tensions TensionLister

	// Learning provides access to learning interaction records.
	Learning LearningLister

	// Patterns provides access to pattern records.
	Patterns PatternLister
}

// TensionLister lists tensions for a workflow.
type TensionLister interface {
	ListForWorkflow(ctx context.Context, workflowID string) ([]archaeodomain.Tension, error)
}

// LearningLister lists learning interactions for a workflow.
type LearningLister interface {
	ListForWorkflow(ctx context.Context, workflowID string) ([]archaeodomain.ExplorationSnapshot, error)
}

// PatternLister lists patterns for a workflow.
type PatternLister interface {
	ListForWorkflow(ctx context.Context, workflowID string) ([]PatternRecord, error)
}

// PatternRecord is a minimal pattern representation.
type PatternRecord struct {
	ID          string
	Title       string
	Description string
	Status      string
}

func (r *SessionResumeResolver) resolveSemanticSummary(ctx context.Context, workflowID string, services SemanticResolutionServices) SessionSemanticSummary {
	var summary SessionSemanticSummary

	// Resolve tensions
	if services.Tensions != nil {
		if tensions, err := services.Tensions.ListForWorkflow(ctx, workflowID); err == nil {
			for _, t := range tensions {
				summary.Tensions = append(summary.Tensions, SemanticFindingSummary{
					ID:      t.ID,
					Title:   t.Kind,
					Summary: t.Description,
					Kind:    "tension",
					Status:  string(t.Status),
				})
			}
		}
	}

	// Resolve learning interactions
	if services.Learning != nil {
		if interactions, err := services.Learning.ListForWorkflow(ctx, workflowID); err == nil {
			for _, li := range interactions {
				summary.LearningInteractions = append(summary.LearningInteractions, SemanticFindingSummary{
					ID:      li.ID,
					Title:   li.Summary,
					Summary: li.Summary,
					Kind:    "learning_interaction",
					Status:  "active",
				})
			}
		}
	}

	return summary
}
