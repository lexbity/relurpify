package promptprovider

import (
	"strings"

	"codeburg.org/lexbit/relurpify/framework/prompt"
)

type reactSkillPolicyProvider struct{}

func (reactSkillPolicyProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if ctx.AgentSpec == nil {
		return prompt.ContextChunk{}
	}
	var parts []string

	// Agent-level prompt override from the spec.
	if p := strings.TrimSpace(ctx.AgentSpec.Prompt); p != "" {
		parts = append(parts, p)
	}

	// Skill verification hints.
	sc := ctx.AgentSpec.SkillConfig
	if len(sc.Verification.SuccessTools) > 0 {
		parts = append(parts, "Verification tools: "+strings.Join(sc.Verification.SuccessTools, ", "))
	}
	if sc.Verification.StopOnSuccess {
		parts = append(parts, "Stop immediately after a successful verification tool runs after the latest edit.")
	}
	if len(sc.Recovery.FailureProbeTools) > 0 {
		parts = append(parts, "Recovery probe tools on failure: "+strings.Join(sc.Recovery.FailureProbeTools, ", "))
	}

	// Tool execution policies.
	for toolName, policy := range ctx.AgentSpec.ToolExecutionPolicy {
		hint := strings.TrimSpace(string(policy.Execute))
		if hint != "" && hint != "allow" {
			parts = append(parts, "Tool policy ["+toolName+"]: "+hint)
		}
	}

	if len(parts) == 0 {
		return prompt.ContextChunk{}
	}
	return prompt.ContextChunk{Content: strings.Join(parts, "\n")}
}

func (reactSkillPolicyProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "react.skill_policy",
		Description: "Renders the agent skill policy: prompt override, verification hints, tool execution policies.",
		Paradigms:   []string{"react"},
	}
}
