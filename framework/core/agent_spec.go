package core

import (
	"fmt"
	"os"
	"strings"
)

// AgentRuntimeSpec describes CLI/runtime level configuration derived from the
// manifest. These fields are optional from the sandbox point of view but
// provide the additional metadata needed by the orchestrator.
type AgentRuntimeSpec struct {
	Implementation    string                `yaml:"implementation" json:"implementation"` // e.g. "react", "planner", "coding"
	Mode              AgentMode             `yaml:"mode" json:"mode"`
	Version           string                `yaml:"version,omitempty" json:"version,omitempty"`
	Prompt            string                `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	Model             AgentModelConfig      `yaml:"model" json:"model"`
	AllowedTools        []string                       `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty"`
	ToolExecutionPolicy map[string]ToolPolicy          `yaml:"tool_execution_policy,omitempty" json:"tool_execution_policy,omitempty"`
	GlobalPolicies      map[string]AgentPermissionLevel `yaml:"policies,omitempty" json:"policies,omitempty"`
	Bash              AgentBashPermissions  `yaml:"bash_permissions,omitempty" json:"bash_permissions,omitempty"`
	Files             AgentFileMatrix       `yaml:"file_permissions,omitempty" json:"file_permissions,omitempty"`
	Invocation        AgentInvocationSpec   `yaml:"invocation,omitempty" json:"invocation,omitempty"`
	Context           AgentContextSpec      `yaml:"context,omitempty" json:"context,omitempty"`
	LSP               AgentLSPSpec          `yaml:"lsp,omitempty" json:"lsp,omitempty"`
	Search            AgentSearchSpec       `yaml:"search,omitempty" json:"search,omitempty"`
	Metadata          AgentMetadata         `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	OllamaToolCalling *bool                 `yaml:"ollama_tool_calling,omitempty" json:"ollama_tool_calling,omitempty"`
	Logging           *AgentLoggingSpec     `yaml:"logging,omitempty" json:"logging,omitempty"`
}

// AgentLSPSpec configures Language Server Protocol features.
type AgentLSPSpec struct {
	Servers map[string]string `yaml:"servers" json:"servers"` // "go": "gopls", "python": "pyright"
	Enabled bool              `yaml:"enabled" json:"enabled"`
	Timeout string            `yaml:"timeout" json:"timeout"`
}

// AgentSearchSpec configures search/indexing capabilities.
type AgentSearchSpec struct {
	HybridEnabled bool `yaml:"hybrid_enabled" json:"hybrid_enabled"` // Use both vector and AST
	VectorIndex   bool `yaml:"vector_index" json:"vector_index"`
	ASTIndex      bool `yaml:"ast_index" json:"ast_index"`
}

// ToolCallingEnabled reports whether Ollama tool calling should be used.
func (a *AgentRuntimeSpec) ToolCallingEnabled() bool {
	if a == nil || a.OllamaToolCalling == nil {
		return true
	}
	return *a.OllamaToolCalling
}

// AgentLoggingSpec controls debug logging toggles for the agent.
type AgentLoggingSpec struct {
	LLM   *bool `yaml:"llm,omitempty" json:"llm,omitempty"`
	Agent *bool `yaml:"agent,omitempty" json:"agent,omitempty"`
}

// AgentMode categorizes the manifest mode.
type AgentMode string

const (
	AgentModePrimary AgentMode = "primary"
	AgentModeSub     AgentMode = "subagent"
	AgentModeSystem  AgentMode = "system"
)

// AgentModelConfig describes an LLM backing the agent.
type AgentModelConfig struct {
	Provider    string  `yaml:"provider" json:"provider"`
	Name        string  `yaml:"name" json:"name"`
	Temperature float64 `yaml:"temperature" json:"temperature"`
	MaxTokens   int     `yaml:"max_tokens" json:"max_tokens"`
}

// ToolPolicy configures visibility and execution gating for a single tool.
// Execution controls whether calls are allowed, denied, or require HITL approval.
type ToolPolicy struct {
	Execute AgentPermissionLevel `yaml:"execute,omitempty" json:"execute,omitempty"` // allow/deny/ask
}

// AgentBashPermissions constrains shell commands.
type AgentBashPermissions struct {
	AllowPatterns []string             `yaml:"allow_patterns" json:"allow_patterns"`
	DenyPatterns  []string             `yaml:"deny_patterns" json:"deny_patterns"`
	Default       AgentPermissionLevel `yaml:"default" json:"default"`
}

// AgentFileMatrix scopes write/edit operations.
type AgentFileMatrix struct {
	Write AgentFilePermissionSet `yaml:"write" json:"write"`
	Edit  AgentFilePermissionSet `yaml:"edit" json:"edit"`
}

// AgentFilePermissionSet stores glob allow/deny rules.
type AgentFilePermissionSet struct {
	AllowPatterns     []string             `yaml:"allow_patterns" json:"allow_patterns"`
	DenyPatterns      []string             `yaml:"deny_patterns" json:"deny_patterns"`
	Default           AgentPermissionLevel `yaml:"default" json:"default"`
	RequireApproval   bool                 `yaml:"require_approval" json:"require_approval"`
	DocumentationOnly bool                 `yaml:"documentation_only" json:"documentation_only"`
}

// AgentInvocationSpec holds recursion data.
type AgentInvocationSpec struct {
	CanInvokeSubagents bool     `yaml:"can_invoke_subagents" json:"can_invoke_subagents"`
	AllowedSubagents   []string `yaml:"allowed_subagents" json:"allowed_subagents"`
	MaxDepth           int      `yaml:"max_depth" json:"max_depth"`
}

// AgentContextSpec limits context window.
type AgentContextSpec struct {
	MaxFiles            int    `yaml:"max_files" json:"max_files"`
	MaxTokens           int    `yaml:"max_tokens" json:"max_tokens"`
	IncludeGitHistory   bool   `yaml:"include_git_history" json:"include_git_history"`
	IncludeDependencies bool   `yaml:"include_dependencies" json:"include_dependencies"`
	CompressionStrategy string `yaml:"compression_strategy" json:"compression_strategy"` // "summary", "truncate", "hybrid"
	ProgressiveLoading  bool   `yaml:"progressive_loading" json:"progressive_loading"`
}

// AgentMetadata captures auxiliary metadata for display.
type AgentMetadata struct {
	Author   string   `yaml:"author" json:"author"`
	Tags     []string `yaml:"tags" json:"tags"`
	Priority int      `yaml:"priority" json:"priority"`
}

// AgentPermissionLevel enumerates allow/deny/ask.
type AgentPermissionLevel string

const (
	AgentPermissionAllow AgentPermissionLevel = "allow"
	AgentPermissionDeny  AgentPermissionLevel = "deny"
	AgentPermissionAsk   AgentPermissionLevel = "ask"
)

// Validate ensures the agent runtime section is well-formed.
func (a *AgentRuntimeSpec) Validate() error {
	if a == nil {
		return nil
	}
	if a.Mode == "" {
		return fmt.Errorf("agent mode required")
	}
	switch a.Mode {
	case AgentModePrimary, AgentModeSub, AgentModeSystem:
	default:
		return fmt.Errorf("invalid agent mode %s", a.Mode)
	}
	if err := a.Model.Validate(); err != nil {
		return fmt.Errorf("model invalid: %w", err)
	}
	for name, policy := range a.ToolExecutionPolicy {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("tool policy contains empty tool name")
		}
		switch policy.Execute {
		case AgentPermissionAllow, AgentPermissionAsk, AgentPermissionDeny, "":
		default:
			return fmt.Errorf("tool policy %s execute=%s invalid", name, policy.Execute)
		}
	}
	for _, tool := range a.AllowedTools {
		if strings.TrimSpace(tool) == "" {
			return fmt.Errorf("allowed_tools contains empty entry")
		}
	}
	if err := a.Files.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate ensures model configuration is provided.
func (m AgentModelConfig) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("model name required")
	}
	if m.Provider == "" {
		return fmt.Errorf("model provider required")
	}
	return nil
}

// Validate ensures file permission sets are consistent.
func (f AgentFileMatrix) Validate() error {
	if err := f.Write.validate("write"); err != nil {
		return err
	}
	if err := f.Edit.validate("edit"); err != nil {
		return err
	}
	return nil
}

func (set AgentFilePermissionSet) validate(label string) error {
	for _, pattern := range append([]string{}, append(set.AllowPatterns, set.DenyPatterns...)...) {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			return fmt.Errorf("%s permission contains empty glob", label)
		}
		if strings.Contains(pattern, string(os.PathSeparator)+string(os.PathSeparator)) {
			return fmt.Errorf("%s permission glob %s invalid", label, pattern)
		}
	}
	switch set.Default {
	case AgentPermissionAllow, AgentPermissionAsk, AgentPermissionDeny, "":
	default:
		return fmt.Errorf("%s permission default %s invalid", label, set.Default)
	}
	return nil
}
