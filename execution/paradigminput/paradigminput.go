package paradigminput

import (
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/prompt"
	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/platform/contracts"
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
	Task                *capability.Task
	Tools               *capability.Registry
	Mode                string
}

func (in *ParadigmInput) BuildRuntimeContext(consumerID string, state map[string]any, env *contextdata.Envelope, caps []capability.CapabilityDescriptor) prompt.RuntimeContext {
	var tools []contracts.Tool
	if in.Tools != nil {
		tools = in.Tools.CallableTools()
	}
	return prompt.RuntimeContext{
		Variables:          extractVariables(in.Task),
		State:              state,
		Envelope:           env,
		Paradigm:           "react",
		ConsumerID:         consumerID,
		Task:               in.Task,
		Tools:              tools,
		Capabilities:       caps,
	}
}

func extractVariables(task *capability.Task) map[string]string {
	if task == nil {
		return nil
	}
	return map[string]string{
		"instruction": task.Instruction,
	}
}
