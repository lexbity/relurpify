package orchestrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	archaeologybehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/archaeology"
	chatbehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/chat"
	debugbehavior "github.com/lexcodex/relurpify/named/euclo/relurpicabilities/debug"
)

type Service struct {
	behaviors map[string]execution.Behavior
}

func NewService() *Service {
	s := &Service{behaviors: map[string]execution.Behavior{}}
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
	return behavior.Execute(ctx, in)
}
