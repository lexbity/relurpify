package security

import "time"

// NetworkRule describes a sandbox network allowance or restriction.
type NetworkRule struct {
	Direction string `yaml:"direction,omitempty"`
	Protocol  string `yaml:"protocol,omitempty"`
	Host      string `yaml:"host,omitempty"`
	Port      int    `yaml:"port,omitempty"`
}

// SandboxPolicy captures the filesystem and network constraints loaded from config.
type SandboxPolicy struct {
	ReadOnlyRoot    bool          `yaml:"read_only_root,omitempty"`
	ProtectedPaths  []string      `yaml:"protected_paths,omitempty"`
	NoNewPrivileges bool          `yaml:"no_new_privileges,omitempty"`
	SeccompProfile  string        `yaml:"seccomp_profile,omitempty"`
	AllowedEnvKeys  []string      `yaml:"allowed_env_keys,omitempty"`
	DeniedEnvKeys   []string      `yaml:"denied_env_keys,omitempty"`
	NetworkRules    []NetworkRule `yaml:"network_rules,omitempty"`
	// ReapOrphans enables boot-time reaping of orphaned managed containers
	// whose owner process is dead (crashed sessions). Default true.
	ReapOrphans bool `yaml:"reap_orphans,omitempty"`
	// OrphanMaxAge is the absolute age cap that reaps a managed container
	// regardless of owner liveness (default 24h).
	OrphanMaxAge time.Duration `yaml:"-"`
	// ImageDigest pins the sandbox runtime image by digest
	// (security/sandbox.policy.yaml `image_digest: sha256:…`).
	ImageDigest string `yaml:"image_digest,omitempty"`
}

// ShellBlacklist stores forbidden shell patterns.
type ShellBlacklist struct {
	Rules []BlacklistRule `yaml:"rules,omitempty"`
}

// BlacklistRule is a single shell deny rule.
type BlacklistRule struct {
	ID      string `yaml:"id,omitempty"`
	Pattern string `yaml:"pattern,omitempty"`
	Reason  string `yaml:"reason,omitempty"`
	Action  string `yaml:"action,omitempty"`
}

// ToolPolicy configures per-tool execution permissions.
type ToolPolicy struct {
	Execute string `yaml:"execute,omitempty"`
}
