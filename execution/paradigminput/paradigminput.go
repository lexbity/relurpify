package paradigminput

import (
	"context"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/governance/policy"
)

type ParadigmInput struct {
	Model               agentspec.AgentModelConfig
	Prompt              string
	StopOnSuccess       bool
	VerificationTools   []string
	RecoveryProbeTools  []string
	ResolvedPolicy      policy.ResolvedAgentPolicy
	InsertionPolicies   []agentspec.CapabilityPolicy
	ToolExecutionPolicy map[string]agentspec.ToolPolicy
	ProviderPolicies    map[string]agentspec.ProviderPolicy
	Task                *execution.Task
	Tools               *registry.CapabilityRegistry
	Mode                string
}

func (in *ParadigmInput) BuildRuntimeContext(consumerID string, state map[string]any, env *contextdata.Envelope, caps []descriptor.CapabilityDescriptor) prompt.RuntimeContext {
	var tools []ports.Tool
	if in.Tools != nil {
		tools = in.Tools.CallableTools(context.Background())
	}
	return prompt.RuntimeContext{
		Variables:    extractVariables(in.Task),
		State:        state,
		Envelope:     env,
		Paradigm:     "react",
		ConsumerID:   consumerID,
		Task:         in.Task,
		Tools:        tools,
		Capabilities: caps,
	}
}

func extractVariables(task *execution.Task) map[string]string {
	if task == nil {
		return nil
	}
	return map[string]string{
		"instruction": task.Instruction,
	}
}
