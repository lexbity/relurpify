package contracts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// CommandRequest captures process execution metadata routed through a sandbox.
type CommandRequest struct {
	Workdir        string
	Args           []string
	Env            []string
	Input          string
	Timeout        time.Duration
	MaxOutputBytes int64         // DEPRECATED prompt cap — removed in Phase 6
	UsePTY         bool          // allocate a pseudo-terminal for the subprocess
	MemoryBytes    int64         // 0 => default (512 MiB)
	PidsLimit      int64         // 0 => default (256)
	CPUs           float64       // 0 => default (1.0)
	OutputCeiling  int64         // 0 => default (32 MiB/stream); teardown trigger
	GracePeriod    time.Duration // SIGTERM→SIGKILL grace (default 3s)
}

// CommandResult is the canonical output of a sandboxed command execution.
type CommandResult struct {
	Stdout      string        // bounded by the artifact-store spill, not a prompt cap
	Stderr      string
	ExitCode    int           // -1 when the process was killed by signal/teardown
	Signaled    bool          // true if terminated by signal (timeout/teardown/OOM)
	TimedOut    bool          // true if the deadline fired
	TornDown    bool          // true if the container was force-removed
	OOMKilled   bool          // true if killed by the output ceiling or cgroup OOM
	Duration    time.Duration
	StdoutBytes int64         // total bytes produced (may exceed len(Stdout) if spilled)
	StderrBytes int64
	StdoutRef   string        // artifact-store handle for full stdout (Phase 5+)
	StderrRef   string        // artifact-store handle for full stderr
}

// CommandRunner describes a primitive capable of executing commands in a sandbox.
type CommandRunner interface {
	Run(ctx context.Context, req CommandRequest) (*CommandResult, error)
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

// CommandRunnerConfig carries the narrow slice of runtime config needed by the
// sandbox layer to configure command execution. It keeps the platform layer
// independent from higher-level configuration loaders.
type CommandRunnerConfig struct {
	Image           string
	RunAsUser       int
	ReadOnlyRoot    bool
	NoNewPrivileges bool
	Workspace       string
}

// SandboxConfig exposes runtime knobs for a sandbox backend.
type SandboxConfig struct {
	RunscPath        string
	ContainerRuntime string // docker or containerd
	Platform         string // ptrace or kvm
	NetworkIsolation bool
	ReadOnlyRoot     bool
	SeccompProfile   string
}

// Capability names describe which security intent a backend can enforce.
type Capability string

const (
	CapabilityNetworkIsolation  Capability = "network_isolation"
	CapabilityReadOnlyRoot      Capability = "read_only_root"
	CapabilityProtectedPaths    Capability = "protected_paths"
	CapabilityNoNewPrivileges   Capability = "no_new_privileges"
	CapabilitySeccomp           Capability = "seccomp"
	CapabilityUserMapping       Capability = "user_mapping"
	CapabilityPerCommandWorkdir Capability = "per_command_workdir"
	CapabilityEnvFiltering      Capability = "env_filtering"
)

// Capabilities reports the enforcement features a backend can actually apply.
type Capabilities struct {
	NetworkIsolation  bool
	ReadOnlyRoot      bool
	ProtectedPaths    bool
	NoNewPrivileges   bool
	Seccomp           bool
	UserMapping       bool
	PerCommandWorkdir bool
	EnvFiltering      bool
}

// Supports reports whether a named backend capability is available.
func (c Capabilities) Supports(cap Capability) bool {
	switch cap {
	case CapabilityNetworkIsolation:
		return c.NetworkIsolation
	case CapabilityReadOnlyRoot:
		return c.ReadOnlyRoot
	case CapabilityProtectedPaths:
		return c.ProtectedPaths
	case CapabilityNoNewPrivileges:
		return c.NoNewPrivileges
	case CapabilitySeccomp:
		return c.Seccomp
	case CapabilityUserMapping:
		return c.UserMapping
	case CapabilityPerCommandWorkdir:
		return c.PerCommandWorkdir
	case CapabilityEnvFiltering:
		return c.EnvFiltering
	default:
		return false
	}
}

// Backend describes a backend-neutral sandbox policy contract. Implementations
// (e.g. platform/sandbox/dockersandbox, framework/sandbox gVisor runtime) live
// at or above the platform layer and depend only on these contract types.
type Backend interface {
	Name() string
	Verify(ctx context.Context) error
	Capabilities() Capabilities
	ValidatePolicy(policy SandboxPolicy) error
	ApplyPolicy(ctx context.Context, policy SandboxPolicy) error
	Policy() SandboxPolicy
}

// SandboxRuntime describes a sandbox backend plus the runtime config required by
// the command runner path.
type SandboxRuntime interface {
	Backend
	RunConfig() SandboxConfig
}

// CommandRunnerProvider lets a sandbox backend supply a specialized runner.
type CommandRunnerProvider interface {
	NewCommandRunner(config *CommandRunnerConfig) (CommandRunner, error)
}

// Validate ensures universal policy invariants hold before backend-specific
// capability checks run.
func (p SandboxPolicy) Validate() error {
	allowed := make(map[string]struct{}, len(p.AllowedEnvKeys))
	for _, key := range p.AllowedEnvKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			return errors.New("allowed env key required")
		}
		if _, ok := allowed[key]; ok {
			return fmt.Errorf("duplicate allowed env key %q", key)
		}
		allowed[key] = struct{}{}
	}
	for _, key := range p.DeniedEnvKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			return errors.New("denied env key required")
		}
		if _, ok := allowed[key]; ok {
			return fmt.Errorf("env key %q cannot be both allowed and denied", key)
		}
	}
	for i, rule := range p.NetworkRules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("network rule %d: %w", i, err)
		}
	}
	for i, path := range p.ProtectedPaths {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("protected path %d required", i)
		}
	}
	return nil
}

// NewCommandResult constructs a CommandResult from raw command output,
// error, and lifecycle state. Used by both sandbox runners.
func NewCommandResult(stdout, stderr string, runErr error, elapsed time.Duration, tornDown bool) *CommandResult {
	res := &CommandResult{
		Stdout:      stdout,
		Stderr:      stderr,
		StdoutBytes: int64(len(stdout)),
		StderrBytes: int64(len(stderr)),
		Duration:    elapsed,
		TornDown:    tornDown,
	}
	if tornDown {
		res.ExitCode = -1
		res.TimedOut = true
		res.Signaled = true
	} else if runErr != nil {
		var exitErr interface{ ExitCode() int }
		if errors.As(runErr, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = -1
		}
	}
	return res
}

// GracePeriodOrDefault returns the effective grace period, defaulting to 3s.
func GracePeriodOrDefault(d time.Duration) time.Duration {
	if d <= 0 {
		return 3 * time.Second
	}
	return d
}

// MemoryBytesOrDefault returns the effective memory limit, defaulting to 512 MiB.
func MemoryBytesOrDefault(d int64) int64 {
	if d <= 0 {
		return 512 * 1024 * 1024 // 512 MiB
	}
	return d
}

// PidsLimitOrDefault returns the effective pids limit, defaulting to 256.
func PidsLimitOrDefault(d int64) int64 {
	if d <= 0 {
		return 256
	}
	return d
}

// CPUsOrDefault returns the effective CPU count, defaulting to 1.0.
func CPUsOrDefault(f float64) float64 {
	if f <= 0 {
		return 1.0
	}
	return f
}

// Validate checks that a network rule is structurally sound.
func (r NetworkRule) Validate() error {
	if strings.TrimSpace(r.Direction) == "" {
		return errors.New("direction required")
	}
	switch strings.ToLower(strings.TrimSpace(r.Direction)) {
	case "egress", "ingress":
	default:
		return fmt.Errorf("unsupported direction %q", r.Direction)
	}
	if strings.TrimSpace(r.Protocol) == "" {
		return errors.New("protocol required")
	}
	if r.Port < 0 {
		return fmt.Errorf("invalid port %d", r.Port)
	}
	return nil
}

// Note: FileScopePolicy is defined in filescope.go
