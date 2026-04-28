package contracts

import (
	"context"
	"time"
)

// CommandRequest captures process execution metadata routed through a sandbox.
type CommandRequest struct {
	Workdir string
	Args    []string
	Env     []string
	Input   string
	Timeout time.Duration
}

// CommandRunner describes a primitive capable of executing commands in a sandbox.
type CommandRunner interface {
	Run(ctx context.Context, req CommandRequest) (stdout string, stderr string, err error)
}

// CommandPolicy decides whether a command request may proceed.
// Implemented by framework/sandbox.CommandPolicy.
type CommandPolicy interface {
	AllowCommand(ctx context.Context, req CommandRequest) error
}

// CommandPolicyFunc adapts a function to CommandPolicy.
type CommandPolicyFunc func(context.Context, CommandRequest) error

// AllowCommand implements CommandPolicy.
func (f CommandPolicyFunc) AllowCommand(ctx context.Context, req CommandRequest) error {
	return f(ctx, req)
}

// NetworkRule defines network access rules for sandbox policies.
type NetworkRule struct {
	Direction   string // "ingress" or "egress"
	Protocol    string // "tcp", "udp", etc.
	Host        string
	Port        int
	Description string
}

// SandboxPolicy captures the backend-neutral security intent to apply to a sandbox runtime.
// Fields are universal unless a backend explicitly rejects them via
// ValidatePolicy.
type SandboxPolicy struct {
	NetworkRules    []NetworkRule
	ReadOnlyRoot    bool
	ProtectedPaths  []string
	NoNewPrivileges bool
	SeccompProfile  string
	AllowedEnvKeys  []string
	DeniedEnvKeys   []string
}

// CommandRunnerConfig carries the narrow slice of manifest data needed by the
// sandbox layer to configure command execution. This struct replaces the
// previous dependency on *manifest.AgentManifest to prevent platform packages
// from importing framework/manifest.
type CommandRunnerConfig struct {
	Image           string
	RunAsUser       int
	ReadOnlyRoot    bool
	NoNewPrivileges bool
	Workspace       string
}

// Note: FileScopePolicy is defined in filescope.go
