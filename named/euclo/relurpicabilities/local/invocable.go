package local

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
)

// Invocable implementations for local routines.

// NewSupportingInvocables returns all supporting invocables for the local package.
func NewSupportingInvocables() []execution.Invocable {
	return []execution.Invocable{
		&deferralsSurfaceInvocable{},
		&learningPromoteInvocable{},
	}
}

// deferralsSurfaceInvocable wraps DeferralsSurfaceRoutine as an Invocable.
type deferralsSurfaceInvocable struct{}

func (d *deferralsSurfaceInvocable) ID() string { return euclorelurpic.CapabilityDeferralsSurface }

func (d *deferralsSurfaceInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := DeferralsSurfaceRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (d *deferralsSurfaceInvocable) IsPrimary() bool { return false }

// deferralsResolveInvocable wraps a DeferralsResolveRoutine as an Invocable.
type deferralsResolveInvocable struct {
	routine *DeferralsResolveRoutine
}

// NewDeferralsResolveInvocable creates a new Invocable for the deferrals resolve routine.
func NewDeferralsResolveInvocable(routine *DeferralsResolveRoutine) execution.Invocable {
	return &deferralsResolveInvocable{routine: routine}
}

func (d *deferralsResolveInvocable) ID() string { return euclorelurpic.CapabilityDeferralsResolve }

func (d *deferralsResolveInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	if d.routine == nil {
		return nil, fmt.Errorf("nil deferrals resolve routine")
	}
	artifacts, err := d.routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (d *deferralsResolveInvocable) IsPrimary() bool { return false }

// learningPromoteInvocable wraps LearningPromoteRoutine as an Invocable.
type learningPromoteInvocable struct{}

func (l *learningPromoteInvocable) ID() string { return euclorelurpic.CapabilityLearningPromote }

func (l *learningPromoteInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	routine := LearningPromoteRoutine{}
	artifacts, err := routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (l *learningPromoteInvocable) IsPrimary() bool { return false }

// learningPromoteInvocableWithDeps wraps a LearningPromoteRoutine with dependencies as an Invocable.
type learningPromoteInvocableWithDeps struct {
	routine *LearningPromoteRoutine
}

// NewLearningPromoteInvocable creates a new Invocable for the learning promote routine with dependencies.
func NewLearningPromoteInvocable(routine *LearningPromoteRoutine) execution.Invocable {
	return &learningPromoteInvocableWithDeps{routine: routine}
}

func (l *learningPromoteInvocableWithDeps) ID() string {
	return euclorelurpic.CapabilityLearningPromote
}

func (l *learningPromoteInvocableWithDeps) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	if l.routine == nil {
		return nil, fmt.Errorf("nil learning promote routine")
	}
	artifacts, err := l.routine.Execute(ctx, convertInvokeInputToRoutineInput(in))
	if err != nil {
		return nil, err
	}
	return &core.Result{
		Success: true,
		Data:    map[string]any{"artifacts": artifacts},
	}, nil
}

func (l *learningPromoteInvocableWithDeps) IsPrimary() bool { return false }

func convertInvokeInputToRoutineInput(in execution.InvokeInput) euclorelurpic.RoutineInput {
	return euclorelurpic.RoutineInput{
		Task:  in.Task,
		State: in.State,
		Work: euclorelurpic.WorkContext{
			PrimaryCapabilityID:             in.Work.PrimaryRelurpicCapabilityID,
			SupportingRelurpicCapabilityIDs: append([]string(nil), in.Work.SupportingRelurpicCapabilityIDs...),
			PatternRefs:                     append([]string(nil), in.Work.SemanticInputs.PatternRefs...),
			TensionRefs:                     append([]string(nil), in.Work.SemanticInputs.TensionRefs...),
			ProspectiveRefs:                 append([]string(nil), in.Work.SemanticInputs.ProspectiveRefs...),
			ConvergenceRefs:                 append([]string(nil), in.Work.SemanticInputs.ConvergenceRefs...),
			RequestProvenanceRefs:           append([]string(nil), in.Work.SemanticInputs.RequestProvenanceRefs...),
		},
		Environment:   in.Environment,
		ServiceBundle: in.ServiceBundle,
	}
}
