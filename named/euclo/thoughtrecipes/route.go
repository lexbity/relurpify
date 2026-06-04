package thoughtrecipe

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/named/euclo/surface"
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

func lowerRouteDecl(decl *RouteDecl, agents map[string]AgentBinding, runIndex *int, plan *ExecutionPlan, inheritedToolScopes []ToolScopeFrame) (*CompiledRouteGroup, error) {
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

		steps, err := lowerRouteExecutionItems(branch.Body, agents, runIndex, plan, inheritedToolScopes)
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

func lowerRouteExecutionItems(items []ExecutionItem, agents map[string]AgentBinding, runIndex *int, plan *ExecutionPlan, inheritedToolScopes []ToolScopeFrame) ([]ExecutionStep, error) {
	type executionFrame struct {
		items []ExecutionItem
		index int
	}

	steps := make([]ExecutionStep, 0)
	stack := []executionFrame{{items: items}}

	for len(stack) > 0 {
		frame := &stack[len(stack)-1]
		if frame.index >= len(frame.items) {
			stack = stack[:len(stack)-1]
			continue
		}

		item := frame.items[frame.index]
		frame.index++

		switch node := item.(type) {
		case *RunDecl:
			step, err := lowerAgentExecutionDecl("run", node.Agent, node.Items, agents, runIndex, inheritedToolScopes)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *DelegateDecl:
			step, err := lowerAgentExecutionDecl("delegate", node.Agent, node.Items, agents, runIndex, inheritedToolScopes)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *RouteDecl:
			group, err := lowerRouteDecl(node, agents, runIndex, plan, inheritedToolScopes)
			if err != nil {
				return nil, err
			}
			plan.Routes = append(plan.Routes, *group)
		case *PipelineDecl:
			for i := len(node.Stages) - 1; i >= 0; i-- {
				stage := node.Stages[i]
				stack = append(stack, executionFrame{items: stage.Body})
			}
		case *DirectiveBlock:
			if len(node.Body) > 0 {
				stack = append(stack, executionFrame{items: node.Body})
			}
		case *AskDecl:
			step, err := lowerAskDecl(node, runIndex)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *CapabilityInvocation:
			step, err := lowerCapabilityExecutionDecl(node, runIndex)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		}
	}
	return steps, nil
}

func lowerAgentExecutionDecl(kind string, agent Identifier, items []ExecutionItem, agents map[string]AgentBinding, index *int, inheritedToolScopes []ToolScopeFrame) (ExecutionStep, error) {
	if index == nil {
		return ExecutionStep{}, fmt.Errorf("execution index is nil")
	}
	agentName := strings.TrimSpace(agent.Value)
	binding, ok := agents[agentName]
	if !ok {
		return ExecutionStep{}, fmt.Errorf("%s:%d:%d: unknown agent %q", agent.GetSpan().Start.File, agent.GetSpan().Start.Line, agent.GetSpan().Start.Column, agentName)
	}
	sources, goals, directives, captures, localToolScopes, promptID, capabilityPlan, config, err := lowerRunItems(items)
	if err != nil {
		return ExecutionStep{}, err
	}
	stepID := fmt.Sprintf("%s.%d.%d.%s.%d", kind, agent.GetSpan().Start.Line, agent.GetSpan().Start.Column, sanitizeComponent(agentName), *index)
	effectiveScopes := appendToolScopeFrames(copyToolScopeFrames(inheritedToolScopes), localToolScopes...)
	step := ExecutionStep{
		ID:                 stepID,
		Type:               kind,
		Paradigm:           binding.Paradigm,
		ToolScopes:         append([]ToolScopeFrame(nil), localToolScopes...),
		EffectiveToolNames: effectiveToolNames(effectiveScopes),
		Goal:               strings.Join(goals, "\n"),
		Sources:            sources,
		Directives:         directives,
		CaptureBindings:    captures,
		PromptID:           promptID,
		Prompt:             strings.Join(goals, "\n"),
		Step:               surface.ThoughtRecipeStep{ID: stepID},
	}
	if capabilityPlan != nil {
		step.CapabilityID = capabilityPlan.CapabilityID
	}
	step.Step.Parent.Paradigm = binding.Paradigm
	step.Step.Parent.Context = surface.ThoughtRecipeStepContext{}
	step.Step.Prompt = step.Prompt
	step.Step.PromptID = promptID
	step.Step.Type = kind
	step.Step.Config = config
	if capabilityPlan != nil {
		if step.Step.Config == nil {
			step.Step.Config = map[string]any{}
		}
		if capabilityPlan.Target != "" {
			step.Step.Config["target"] = capabilityPlan.Target
		}
		if capabilityPlan.Input != "" {
			step.Step.Config["input"] = capabilityPlan.Input
		}
		step.Step.Config["capability_id"] = capabilityPlan.CapabilityID
	}
	if step.Step.Config == nil && (len(step.ToolScopes) > 0 || len(step.EffectiveToolNames) > 0) {
		step.Step.Config = map[string]any{}
	}
	if step.Step.Config != nil {
		if len(step.ToolScopes) > 0 {
			step.Step.Config["tool_scopes"] = summarizeToolScopeFrames(step.ToolScopes)
		}
		if len(step.EffectiveToolNames) > 0 {
			step.Step.Config["effective_tool_names"] = append([]string(nil), step.EffectiveToolNames...)
		}
	}
	*index = *index + 1
	return step, nil
}

func lowerCapabilityExecutionDecl(inv *CapabilityInvocation, index *int) (ExecutionStep, error) {
	if index == nil {
		return ExecutionStep{}, fmt.Errorf("execution index is nil")
	}
	plan, err := LowerCapabilityInvocation(inv)
	if err != nil {
		return ExecutionStep{}, err
	}
	stepID := fmt.Sprintf("capability.%d.%d.%d", inv.GetSpan().Start.Line, inv.GetSpan().Start.Column, *index)
	step := ExecutionStep{
		ID:           stepID,
		Type:         "capability",
		Paradigm:     "euclo",
		CapabilityID: plan.CapabilityID,
		Prompt:       fmt.Sprintf("do relurpic:%s", strings.TrimSpace(inv.Capability.Value)),
		Step: surface.ThoughtRecipeStep{
			ID:   stepID,
			Type: "capability",
			Parent: surface.ThoughtRecipeStepAgent{
				Paradigm: "euclo",
			},
			Config: map[string]any{},
		},
	}
	step.Step.Prompt = step.Prompt
	step.Step.Config["capability_id"] = plan.CapabilityID
	if plan.Target != "" {
		step.Step.Config["target"] = plan.Target
	}
	if plan.Input != "" {
		step.Step.Config["input"] = plan.Input
	}
	*index = *index + 1
	return step, nil
}

func routeGroupID(decl *RouteDecl) string {
	span := decl.GetSpan()
	return fmt.Sprintf("route.%d.%d", span.Start.Line, span.Start.Column)
}
