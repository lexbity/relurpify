package orchestrate

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

type Service struct {
	behaviors map[string]execution.Behavior
	routines  map[string]euclorelurpic.SupportingRoutine
}

func NewService() *Service {
	s := &Service{behaviors: map[string]execution.Behavior{}, routines: map[string]euclorelurpic.SupportingRoutine{}}
	for _, behavior := range []execution.Behavior{
		chatbehavior.NewAskBehavior(),
		chatbehavior.NewInspectBehavior(),
		chatbehavior.NewImplementBehavior(),
		debugbehavior.NewInvestigateBehavior(),
		archaeologybehavior.NewExploreBehavior(),
		archaeologybehavior.NewCompilePlanBehavior(),
		archaeologybehavior.NewImplementPlanBehavior(),
	} {
		s.behaviors[behavior.ID()] = behavior
	}
	for _, routine := range append(append(chatbehavior.NewSupportingRoutines(), debugbehavior.NewSupportingRoutines()...), archaeologybehavior.NewSupportingRoutines()...) {
		s.routines[routine.ID()] = routine
	}
	return s
}

func (s *Service) Execute(ctx context.Context, in execution.ExecuteInput) (*core.Result, error) {
	if s == nil {
		return nil, fmt.Errorf("relurpic behavior service unavailable")
	}
	behaviorID := strings.TrimSpace(in.Work.PrimaryRelurpicCapabilityID)
	behavior, ok := s.behaviors[behaviorID]
	if !ok {
		return nil, fmt.Errorf("relurpic behavior %q unavailable", behaviorID)
	}
	in.RunSupportingRoutine = s.ExecuteRoutine
	return behavior.Execute(ctx, in)
}

func (s *Service) ExecuteRoutine(ctx context.Context, routineID string, task *core.Task, state *core.Context, work runtimepkg.UnitOfWork, env agentenv.AgentEnvironment) ([]euclotypes.Artifact, error) {
	if s == nil {
		return nil, fmt.Errorf("relurpic behavior service unavailable")
	}
	routineID = strings.TrimSpace(routineID)
	routine, ok := s.routines[routineID]
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
		Environment: env,
	})
}
