package security

// NetworkRule describes a sandbox network allowance or restriction.
type NetworkRule struct {
	Direction string `yaml:"direction,omitempty"`
	Protocol  string `yaml:"protocol,omitempty"`
	Host      string `yaml:"host,omitempty"`
	Port      int    `yaml:"port,omitempty"`
}

// SandboxPolicy captures the filesystem and network constraints loaded from config.
type SandboxPolicy struct {
	ReadOnlyRoot    bool         `yaml:"read_only_root,omitempty"`
	ProtectedPaths  []string     `yaml:"protected_paths,omitempty"`
	NoNewPrivileges bool         `yaml:"no_new_privileges,omitempty"`
	SeccompProfile  string       `yaml:"seccomp_profile,omitempty"`
	AllowedEnvKeys  []string     `yaml:"allowed_env_keys,omitempty"`
	DeniedEnvKeys   []string     `yaml:"denied_env_keys,omitempty"`
	NetworkRules    []NetworkRule `yaml:"network_rules,omitempty"`
}

// ShellBlacklist stores forbidden shell patterns.
type ShellBlacklist struct {
	Rules []BlacklistRule `yaml:"rules,omitempty"`
}

// BlacklistRule is a single shell deny rule.
type BlacklistRule struct {
	Pattern string `yaml:"pattern,omitempty"`
	Reason  string `yaml:"reason,omitempty"`
}

// ToolPolicy configures per-tool execution permissions.
type ToolPolicy struct {
	Execute string `yaml:"execute,omitempty"`
}
