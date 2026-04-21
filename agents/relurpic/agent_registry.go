package relurpic

import (
	"fmt"
	"strings"
	"sync"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
)

var customAgentHandlers sync.Map

func RegisterAgentCapabilities(registry *capability.Registry, env agentenv.AgentEnvironment) error {
	if registry == nil {
		return nil
	}
	for _, agentType := range []string{"react", "architect", "pipeline", "planner", "reflection", "chainer", "htn", "blackboard", "rewoo", "goalcon"} {
		id := "agent:" + agentType
		if registry.HasCapability(id) {
			continue
		}
		handler := &AgentCapabilityHandler{
			env:       env,
			agentType: agentType,
			policy:    DefaultInvocationPolicies[agentType],
		}
		if err := registry.RegisterInvocableCapability(handler); err != nil {
			return err
		}
	}
	return nil
}

func RegisterCustomAgentHandler(registry *capability.Registry, id string, handler core.InvocableCapabilityHandler) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("handler id required")
	}
	if handler == nil {
		return fmt.Errorf("handler required")
	}
	customAgentHandlers.Store(id, handler)
	if registry != nil && !registry.HasCapability(id) {
		return registry.RegisterInvocableCapability(handler)
	}
	return nil
}

func agentCapabilityDescriptor(agentType string, policy core.AgentInvocationPolicy) core.CapabilityDescriptor {
	annotations := map[string]any{
		"relurpic_capability": true,
		"workflow":            agentType,
		"agent_type":          agentType,
		"policy": map[string]any{
			"memory_mode": policy.MemoryMode,
			"state_mode":  policy.StateMode,
			"tool_scope":  policy.ToolScope,
		},
	}
	return coordinatedRelurpicDescriptor(
		"agent:"+agentType,
		"agent."+agentType,
		fmt.Sprintf("Execute the %s agent paradigm as an invocable capability.", agentType),
		core.CapabilityKindTool,
		coordinationRoleForAgentType(agentType),
		[]string{"delegate", "compose"},
		[]core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
		structuredTaskSchema("instruction"),
		structuredObjectSchema(map[string]*core.Schema{
			"success": {Type: "boolean"},
			"node_id": {Type: "string"},
		}),
		annotations,
		[]core.RiskClass{core.RiskClassExecute},
		[]core.EffectClass{core.EffectClassContextInsertion},
	)
}

func coordinationRoleForAgentType(agentType string) core.CoordinationRole {
	switch agentType {
	case "planner":
		return core.CoordinationRolePlanner
	case "architect":
		return core.CoordinationRoleArchitect
	case "reflection":
		return core.CoordinationRoleReviewer
	default:
		return core.CoordinationRoleExecutor
	}
}
