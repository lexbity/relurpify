package archaeology

import (
	"context"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
)

type exploreBehavior struct{}
type compilePlanBehavior struct{}
type implementPlanBehavior struct{}

func NewExploreBehavior() execution.Behavior       { return exploreBehavior{} }
func NewCompilePlanBehavior() execution.Behavior   { return compilePlanBehavior{} }
func NewImplementPlanBehavior() execution.Behavior { return implementPlanBehavior{} }

func (exploreBehavior) ID() string { return Explore }

func (exploreBehavior) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	routines := execution.SupportingIDs(in.Work, "euclo:archaeology.")
	for _, routine := range routines {
		execution.EnsureRoutineArtifacts(in.State, routine, in.Work)
	}
	execution.AppendDiagnostic(in.State, "euclo.plan_candidates", "archaeology exploration behavior executed with archaeology-backed semantic inputs")
	execution.SetBehaviorTrace(in.State, in.Work, routines)
	return execution.ExecuteWorkflow(ctx, in)
}

func (compilePlanBehavior) ID() string { return CompilePlan }

func (compilePlanBehavior) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	routines := execution.SupportingIDs(in.Work, "euclo:archaeology.")
	for _, routine := range routines {
		execution.EnsureRoutineArtifacts(in.State, routine, in.Work)
	}
	execution.SetBehaviorTrace(in.State, in.Work, routines)

	planResult, _, err := execution.ExecutePlannerTask(ctx, in, "archaeology-compile-plan",
		"Compile an executable plan for: "+execution.CapabilityTaskInstruction(in.Task))
	if err != nil || planResult == nil || !planResult.Success {
		payload := execution.CompilePlanFallback(in.Work)
		if payload == nil {
			return &core.Result{Success: false, Error: err}, err
		}
		if in.State != nil {
			in.State.Set("pipeline.plan", payload)
		}
		return execution.SuccessResult("archaeology compile-plan produced fallback plan from semantic inputs", []euclotypes.Artifact{{
			ID:         "archaeology_compile_plan",
			Kind:       euclotypes.ArtifactKindPlan,
			Summary:    "compiled plan synthesized from semantic inputs",
			Payload:    payload,
			ProducerID: in.Work.PrimaryRelurpicCapabilityID,
			Status:     "produced",
		}})
	}
	payload := planResult.Data
	if existing := planArtifactFromState(in.State); existing != nil {
		payload = existing
	}
	if in.State != nil {
		in.State.Set("pipeline.plan", payload)
	}
	artifacts := []euclotypes.Artifact{{
		ID:         "archaeology_compile_plan",
		Kind:       euclotypes.ArtifactKindPlan,
		Summary:    execution.ResultSummary(planResult),
		Payload:    payload,
		ProducerID: in.Work.PrimaryRelurpicCapabilityID,
		Status:     "produced",
	}}
	execution.MergeStateArtifactsToContext(in.State, artifacts)
	return execution.SuccessResult("archaeology compile-plan completed successfully", artifacts)
}

func (implementPlanBehavior) ID() string { return ImplementPlan }

func (implementPlanBehavior) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	routines := execution.SupportingIDs(in.Work, "euclo:archaeology.")
	for _, routine := range routines {
		execution.EnsureRoutineArtifacts(in.State, routine, in.Work)
	}
	execution.AppendDiagnostic(in.State, "pipeline.plan", "archaeology implement-plan executing against a compiled plan")
	execution.SetBehaviorTrace(in.State, in.Work, routines)
	return execution.ExecuteWorkflow(ctx, in)
}

func planArtifactFromState(state *core.Context) map[string]any {
	if state == nil {
		return nil
	}
	if raw, ok := state.Get("pipeline.plan"); ok {
		if typed, ok := raw.(map[string]any); ok && len(typed) > 0 {
			return typed
		}
	}
	if raw, ok := state.Get("propose.items"); ok && raw != nil {
		return map[string]any{"items": raw}
	}
	return nil
}
