// Package ports defines consumer-owned interfaces for cross-domain
// communication. SandboxRuntime is the governance-owned port for
// sandbox backends; platform/sandbox implements it.
package ports

import "context"

// SandboxConfig describes sandbox runtime configuration.
type SandboxConfig struct {
	RunscPath        string
	ContainerRuntime string
	Platform         string
	NetworkIsolation bool
	ReadOnlyRoot     bool
	SeccompProfile   string
}

// SandboxPolicy captures the backend-neutral security intent for a sandbox runtime.
type SandboxPolicy struct {
	NetworkRules    []SandboxNetworkRule
	ReadOnlyRoot    bool
	ProtectedPaths  []string
	NoNewPrivileges bool
	SeccompProfile  string
	AllowedEnvKeys  []string
	DeniedEnvKeys   []string
}

// SandboxNetworkRule defines network access rules for sandbox policies.
type SandboxNetworkRule struct {
	Direction   string
	Protocol    string
	Host        string
	Port        int
	Description string
}

// SandboxRuntime describes a sandbox runtime with policy methods.
type SandboxRuntime interface {
	Verify(ctx context.Context) error
	ValidatePolicy(policy SandboxPolicy) error
	ApplyPolicy(ctx context.Context, policy SandboxPolicy) error
	Policy() SandboxPolicy
	RunConfig() SandboxConfig
	Name() string
}

// SandboxSelector chooses a sandbox backend implementation.
type SandboxSelector interface {
	SelectBackend(ctx context.Context, backend string, cfg SandboxConfig, image, workspace string) (SandboxRuntime, error)
	SupportedBackends() []string
}
