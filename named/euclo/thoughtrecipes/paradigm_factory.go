package thoughtrecipes

import (
	"fmt"

	"github.com/lexcodex/relurpify/agents/architect"
	"github.com/lexcodex/relurpify/agents/blackboard"
	"github.com/lexcodex/relurpify/agents/chainer"
	"github.com/lexcodex/relurpify/agents/goalcon"
	"github.com/lexcodex/relurpify/agents/htn"
	"github.com/lexcodex/relurpify/agents/planner"
	"github.com/lexcodex/relurpify/agents/react"
	"github.com/lexcodex/relurpify/agents/reflection"
	"github.com/lexcodex/relurpify/agents/rewoo"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/graph"
)

// ParadigmFactory provides construction and delegation wiring for agent paradigms.
type ParadigmFactory struct{}

// NewParadigmFactory creates a new paradigm factory.
func NewParadigmFactory() *ParadigmFactory {
	return &ParadigmFactory{}
}

// BuildParadigmAgent constructs an agent for the given paradigm name.
// If child is non-nil, it will be wired into the parent agent's delegation slot
// (if the paradigm supports delegation).
func (f *ParadigmFactory) BuildParadigmAgent(
	paradigm string,
	child graph.WorkflowExecutor,
	env agentenv.AgentEnvironment,
	reg *capability.Registry,
) (graph.WorkflowExecutor, error) {
	// Validate required parameters
	if reg == nil {
		return nil, fmt.Errorf("capability registry is required")
	}

	// Build filtered environment using the provided registry
	envWithReg := envWithRegistry(env, reg)

	switch paradigm {
	case "react":
		// React has no delegation slot - child is ignored if provided
		return react.New(envWithReg), nil

	case "planner":
		// Planner has no delegation slot - child is ignored if provided
		return planner.New(envWithReg), nil

	case "rewoo":
		// ReWOO has no delegation slot - child is ignored if provided
		return rewoo.New(envWithReg), nil

	case "htn":
		// HTN has a PrimitiveExec delegation slot
		if child != nil {
			return htn.New(envWithReg, nil, htn.WithPrimitiveExec(child)), nil
		}
		return htn.New(envWithReg, nil), nil

	case "reflection":
		// Reflection has a Delegate slot
		return reflection.New(envWithReg, child), nil

	case "goalcon":
		// GoalCon has a PlanExecutor field
		agent := &goalcon.GoalConAgent{
			Operators: goalcon.DefaultOperatorRegistry(),
		}
		_ = agent.InitializeEnvironment(envWithReg)
		if child != nil {
			agent.PlanExecutor = child
		}
		return agent, nil

	case "blackboard":
		// Blackboard has Sources []KnowledgeSource - child not supported in V1
		if child != nil {
			// Log a warning but don't fail - child is ignored
			// Warning will be returned to caller
		}
		agent := blackboard.New(envWithReg)
		return agent, nil

	case "architect":
		// Architect internal execution is hardcoded - child not supported in V1
		if child != nil {
			// Log a warning but don't fail - child is ignored
		}
		// Architect has internal react + planner composition
		return architect.New(envWithReg), nil

	case "chainer":
		// Chainer has Chain *Chain - child not supported in V1
		if child != nil {
			// Log a warning but don't fail - child is ignored
		}
		agent := chainer.New(envWithReg)
		return agent, nil

	default:
		return nil, fmt.Errorf("unknown paradigm: %s", paradigm)
	}
}

// BuildStepAgent builds an agent for an ExecutionStep.
// This applies the step's capability restrictions and wires any child agent.
func (f *ParadigmFactory) BuildStepAgent(
	step ExecutionStep,
	globalReg *capability.Registry,
	env agentenv.AgentEnvironment,
) (parent graph.WorkflowExecutor, child graph.WorkflowExecutor, warnings []string, err error) {
	// Build child first (if any)
	if step.Child != nil {
		childReg := applyCapabilityFilter(globalReg, step.Child.Capabilities)
		child, err = f.BuildParadigmAgent(step.Child.Paradigm, nil, env, childReg)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("building child agent: %w", err)
		}
	}

	// Check if parent supports delegation but child was declared for non-delegating paradigm
	supportsDelegation := isParadigmWithDelegation(step.Parent.Paradigm)
	if step.Child != nil && !supportsDelegation {
		warnings = append(warnings,
			fmt.Sprintf("child declared for paradigm %s which has no delegation slot; ignoring child",
				step.Parent.Paradigm))
	}

	// Build parent with child wired in (if supported)
	parentReg := applyCapabilityFilter(globalReg, step.Parent.Capabilities)
	var parentChild graph.WorkflowExecutor
	if supportsDelegation {
		parentChild = child
	}
	parent, err = f.BuildParadigmAgent(step.Parent.Paradigm, parentChild, env, parentReg)
	if err != nil {
		return nil, nil, warnings, fmt.Errorf("building parent agent: %w", err)
	}

	return parent, child, warnings, nil
}

// BuildFallbackAgent builds a fallback agent for a step.
func (f *ParadigmFactory) BuildFallbackAgent(
	fallback ExecutionStepAgent,
	globalReg *capability.Registry,
	env agentenv.AgentEnvironment,
) (graph.WorkflowExecutor, error) {
	reg := applyCapabilityFilter(globalReg, fallback.Capabilities)
	return f.BuildParadigmAgent(fallback.Paradigm, nil, env, reg)
}

// isParadigmWithDelegation returns true if the paradigm supports child delegation.
func isParadigmWithDelegation(paradigm string) bool {
	switch paradigm {
	case "htn", "reflection", "goalcon":
		return true
	default:
		return false
	}
}

// applyCapabilityFilter returns a scoped registry view restricted to the allowed IDs.
// When allowedIDs is empty the base registry is returned unchanged.
// The returned *capability.Registry enforces the allowlist in ModelCallableTools,
// CaptureExecutionCatalogSnapshot, and InvokeCapability.
func applyCapabilityFilter(base *capability.Registry, allowedIDs []string) *capability.Registry {
	if len(allowedIDs) == 0 {
		return base
	}
	return base.WithAllowlist(allowedIDs)
}

// envWithRegistry returns an environment with the given registry.
// This uses the environment's WithRegistry method if available.
func envWithRegistry(env agentenv.AgentEnvironment, reg *capability.Registry) agentenv.AgentEnvironment {
	return env.WithRegistry(reg)
}
