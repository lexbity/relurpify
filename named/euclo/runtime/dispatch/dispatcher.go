package dispatch

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	archaeologybehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/archaeology"
	chatbehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/chat"
	debugbehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/debug"
	runtimepkg "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type Dispatcher struct {
	behaviors map[string]execution.Behavior
	routines  map[string]euclorelurpic.SupportingRoutine
}

func NewDispatcher() *Dispatcher {
	d := &Dispatcher{behaviors: map[string]execution.Behavior{}, routines: map[string]euclorelurpic.SupportingRoutine{}}
	for _, behavior := range []execution.Behavior{
		chatbehavior.NewAskBehavior(),
		chatbehavior.NewInspectBehavior(),
		chatbehavior.NewImplementBehavior(),
		debugbehavior.NewInvestigateBehavior(),
		archaeologybehavior.NewExploreBehavior(),
		archaeologybehavior.NewCompilePlanBehavior(),
		archaeologybehavior.NewImplementPlanBehavior(),
	} {
		d.behaviors[behavior.ID()] = behavior
	}
	for _, routine := range append(append(chatbehavior.NewSupportingRoutines(), debugbehavior.NewSupportingRoutines()...), archaeologybehavior.NewSupportingRoutines()...) {
		d.routines[routine.ID()] = routine
	}
	return d
}

func (d *Dispatcher) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	if d == nil {
		return nil, fmt.Errorf("relurpic behavior service unavailable")
	}
	behaviorID := strings.TrimSpace(in.Work.PrimaryRelurpicCapabilityID)
	behavior, ok := d.behaviors[behaviorID]
	if !ok {
		return nil, fmt.Errorf("relurpic behavior %q unavailable", behaviorID)
	}
	in.RunSupportingRoutine = d.ExecuteRoutine
	return behavior.Execute(ctx, in)
}

func (d *Dispatcher) ExecuteRoutine(ctx context.Context, routineID string, task *core.Task, state *core.Context, work runtimepkg.UnitOfWork, env agentenv.AgentEnvironment, bundle execution.ServiceBundle) ([]euclotypes.Artifact, error) {
	if d == nil {
		return nil, fmt.Errorf("relurpic behavior service unavailable")
	}
	routineID = strings.TrimSpace(routineID)
	routine, ok := d.routines[routineID]
	if !ok {
		return nil, nil
	}
	return routine.Execute(ctx, euclorelurpic.RoutineInput{
		Task:  task,
		State: state,
		Work: euclorelurpic.WorkContext{
			PrimaryCapabilityID:             work.PrimaryRelurpicCapabilityID,
			SupportingRelurpicCapabilityIDs: append([]string(nil), work.SupportingRelurpicCapabilityIDs...),
			PatternRefs:                     append([]string(nil), work.SemanticInputs.PatternRefs...),
			TensionRefs:                     append([]string(nil), work.SemanticInputs.TensionRefs...),
			ProspectiveRefs:                 append([]string(nil), work.SemanticInputs.ProspectiveRefs...),
			ConvergenceRefs:                 append([]string(nil), work.SemanticInputs.ConvergenceRefs...),
			RequestProvenanceRefs:           append([]string(nil), work.SemanticInputs.RequestProvenanceRefs...),
		},
		Environment:   env,
		ServiceBundle: bundle,
	})
}
