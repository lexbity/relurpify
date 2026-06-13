package config

// AgentPermissionLevel enumerates allow/deny/ask for permission decisions.
type AgentPermissionLevel string

const (
	AgentPermissionAllow AgentPermissionLevel = "allow"
	AgentPermissionDeny  AgentPermissionLevel = "deny"
	AgentPermissionAsk   AgentPermissionLevel = "ask"
)

// AgentBashPermissions constrains shell commands.
type AgentBashPermissions struct {
	AllowPatterns []string             `yaml:"allow_patterns" json:"allow_patterns"`
	DenyPatterns  []string             `yaml:"deny_patterns" json:"deny_patterns"`
	Default       AgentPermissionLevel `yaml:"default" json:"default"`
}

// AgentLoggingSpec controls debug logging toggles for the agent.
type AgentLoggingSpec struct {
	LLM   *bool `yaml:"llm,omitempty" json:"llm,omitempty"`
	Agent *bool `yaml:"agent,omitempty" json:"agent,omitempty"`
}

// ProviderPolicy configures activation defaults and trust metadata for provider-backed capabilities.
type ProviderPolicy struct {
	Activate               AgentPermissionLevel `yaml:"activate,omitempty" json:"activate,omitempty"`
	DefaultTrust           string               `yaml:"default_trust,omitempty" json:"default_trust,omitempty"`
	AllowCredentialSharing bool                 `yaml:"allow_credential_sharing,omitempty" json:"allow_credential_sharing,omitempty"`
}

// ToolPolicy configures visibility and execution gating for a single tool.
type ToolPolicy struct {
	Execute AgentPermissionLevel `yaml:"execute,omitempty" json:"execute,omitempty"`
}

// AgentModelConfig describes an LLM backing the agent.
type AgentModelConfig struct {
	Provider    string  `yaml:"provider" json:"provider"`
	Name        string  `yaml:"name" json:"name"`
	Temperature float64 `yaml:"temperature" json:"temperature"`
	MaxTokens   int     `yaml:"max_tokens" json:"max_tokens"`
}

// AgentSpec describes the runtime-facing agent configuration loaded from relurpify_cfg.
type AgentSpec struct {
	Implementation      string                    `yaml:"implementation,omitempty" json:"implementation,omitempty"`
	Version             string                    `yaml:"version,omitempty" json:"version,omitempty"`
	Prompt              string                    `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	Model               AgentModelConfig          `yaml:"model" json:"model"`
	ToolExecutionPolicy map[string]ToolPolicy     `yaml:"tool_execution_policy,omitempty" json:"tool_execution_policy,omitempty"`
	ProviderPolicies    map[string]ProviderPolicy `yaml:"provider_policies,omitempty" json:"provider_policies,omitempty"`
	Bash                AgentBashPermissions      `yaml:"bash_permissions,omitempty" json:"bash_permissions,omitempty"`
	NativeToolCalling   *bool                     `yaml:"native_tool_calling,omitempty" json:"native_tool_calling,omitempty"`
	Logging             *AgentLoggingSpec         `yaml:"logging,omitempty" json:"logging,omitempty"`
}

// NativeToolCallingEnabled reports whether native tool calling should be used.
func (a *AgentSpec) NativeToolCallingEnabled() bool {
	if a == nil {
		return true
	}
	if a.NativeToolCalling != nil {
		return *a.NativeToolCalling
	}
	return true
}
