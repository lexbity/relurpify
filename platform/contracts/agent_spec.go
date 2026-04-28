package contracts

// AgentFileMatrix stores file permission rules for agent write/edit operations.
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

// AgentRuntimeSpec describes CLI/runtime level configuration for agents.
// This is a simplified version for platform tool use.
type AgentRuntimeSpec struct {
	Implementation string          `yaml:"implementation,omitempty" json:"implementation,omitempty"`
	Files          AgentFileMatrix `yaml:"file_permissions,omitempty" json:"file_permissions,omitempty"`
}
