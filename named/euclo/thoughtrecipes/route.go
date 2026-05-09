package thoughtrecipe

import (
	"fmt"
	"strings"
)

// NormalizeRoutePredicate converts a parsed predicate into the constrained
// runtime form used by route lowering.
func NormalizeRoutePredicate(pred PredicateExpr) (*RoutePredicate, error) {
	kind := strings.TrimSpace(pred.Kind)
	switch kind {
	case "missing", "present", "is", "contains", "confidence_below":
	default:
		return nil, fmt.Errorf("%s:%d:%d: unsupported route predicate %q", pred.GetSpan().Start.File, pred.GetSpan().Start.Line, pred.GetSpan().Start.Column, pred.Kind)
	}

	return &RoutePredicate{
		positioned: positioned{Span: pred.GetSpan()},
		Raw:        strings.TrimSpace(pred.Raw),
		Kind:       kind,
		Subject:    pred.Subject,
		Operator:   strings.TrimSpace(pred.Operator),
		Value:      pred.Value,
	}, nil
}

func lowerRouteDecl(decl *RouteDecl, agents map[string]AgentBinding, runIndex *int, plan *ExecutionPlan) (*CompiledRouteGroup, error) {
	if decl == nil {
		return nil, fmt.Errorf("route declaration is nil")
	}
	if plan == nil {
		return nil, fmt.Errorf("execution plan is nil")
	}

	group := &CompiledRouteGroup{
		Group: &RouteGroup{
			positioned: positioned{Span: decl.GetSpan()},
			ID:         routeGroupID(decl),
		},
		Branches: make([]CompiledRouteBranch, 0, len(decl.Branches)),
	}

	var hasFallback bool
	for branchIndex, branch := range decl.Branches {
		compiled := CompiledRouteBranch{IsElse: branch.IsElse}
		if branch.IsElse {
			hasFallback = true
		} else {
			pred, err := NormalizeRoutePredicate(branch.Predicate)
			if err != nil {
				return nil, err
			}
			compiled.Predicate = pred
		}

		steps, err := lowerRouteExecutionItems(branch.Body, agents, runIndex, plan)
		if err != nil {
			return nil, err
		}
		compiled.Steps = steps
		if len(steps) == 0 && !branch.IsElse {
			plan.Warnings = append(plan.Warnings, SemanticWarning{
				Span:    branch.GetSpan(),
				Message: fmt.Sprintf("route branch %d has no executable steps", branchIndex+1),
			})
		}
		group.Branches = append(group.Branches, compiled)
	}

	if !hasFallback {
		plan.Warnings = append(plan.Warnings, SemanticWarning{
			Span:    decl.GetSpan(),
			Message: "route has no otherwise branch; unmatched input becomes a no-op",
		})
	}

	return group, nil
}

func lowerRouteExecutionItems(items []ExecutionItem, agents map[string]AgentBinding, runIndex *int, plan *ExecutionPlan) ([]ExecutionStep, error) {
	steps := make([]ExecutionStep, 0)
	for _, item := range items {
		switch node := item.(type) {
		case *RunDecl:
			step, err := lowerAgentExecutionDecl("run", node.Agent, node.Items, agents, runIndex)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *DelegateDecl:
			step, err := lowerAgentExecutionDecl("delegate", node.Agent, node.Items, agents, runIndex)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *RouteDecl:
			group, err := lowerRouteDecl(node, agents, runIndex, plan)
			if err != nil {
				return nil, err
			}
			plan.Routes = append(plan.Routes, *group)
		case *PipelineDecl:
			for _, stage := range node.Stages {
				nested, err := lowerRouteExecutionItems(stage.Body, agents, runIndex, plan)
				if err != nil {
					return nil, err
				}
				steps = append(steps, nested...)
			}
		case *DirectiveBlock:
			nested, err := lowerRouteExecutionItems(node.Body, agents, runIndex, plan)
			if err != nil {
				return nil, err
			}
			steps = append(steps, nested...)
		case *AskDecl:
			step, err := lowerAskDecl(node, runIndex)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		}
	}
	return steps, nil
}

func lowerAgentExecutionDecl(kind string, agent Identifier, items []ExecutionItem, agents map[string]AgentBinding, index *int) (ExecutionStep, error) {
	if index == nil {
		return ExecutionStep{}, fmt.Errorf("execution index is nil")
	}
	agentName := strings.TrimSpace(agent.Value)
	binding, ok := agents[agentName]
	if !ok {
		return ExecutionStep{}, fmt.Errorf("%s:%d:%d: unknown agent %q", agent.GetSpan().Start.File, agent.GetSpan().Start.Line, agent.GetSpan().Start.Column, agentName)
	}
	sources, goals, directives, captures, config := lowerRunItems(items)
	stepID := fmt.Sprintf("%s.%d.%d.%s.%d", kind, agent.GetSpan().Start.Line, agent.GetSpan().Start.Column, sanitizeComponent(agentName), *index)
	step := ExecutionStep{
		ID:              stepID,
		Type:            kind,
		Paradigm:        binding.Paradigm,
		Goal:            strings.Join(goals, "\n"),
		Sources:         sources,
		Directives:      directives,
		CaptureBindings: captures,
		Prompt:          strings.Join(goals, "\n"),
		Step:            ThoughtRecipeStep{ID: stepID},
	}
	step.Step.Parent.Paradigm = binding.Paradigm
	step.Step.Parent.Context = ThoughtRecipeStepContext{}
	step.Step.Prompt = step.Prompt
	step.Step.Type = kind
	step.Step.Config = config
	*index = *index + 1
	return step, nil
}

func routeGroupID(decl *RouteDecl) string {
	span := decl.GetSpan()
	return fmt.Sprintf("route.%d.%d", span.Start.Line, span.Start.Column)
}
