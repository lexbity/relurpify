package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

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

// Backend describes a backend-neutral sandbox policy contract.
type Backend interface {
	Name() string
	Verify(ctx context.Context) error
	Capabilities() Capabilities
	ValidatePolicy(policy SandboxPolicy) error
	ApplyPolicy(ctx context.Context, policy SandboxPolicy) error
	Policy() SandboxPolicy
}

// SandboxRuntime describes a sandbox backend plus the runtime config required by the
// command runner path.
type SandboxRuntime interface {
	Backend
	RunConfig() SandboxConfig
	// Policy returns the currently enforced sandbox policy.
	Policy() SandboxPolicy
}

// SandboxConfig exposes runtime knobs.
type SandboxConfig struct {
	RunscPath        string
	ContainerRuntime string // docker or containerd
	Platform         string // ptrace or kvm
	NetworkIsolation bool
	ReadOnlyRoot     bool
	SeccompProfile   string
}

// NetworkRule represents an allowed network scope.
type NetworkRule struct {
	Direction string
	Protocol  string
	Host      string
	Port      int
}

// SandboxRuntimeImpl enforces runsc-backed execution.
type SandboxRuntimeImpl struct {
	config   SandboxConfig
	verified bool
	mu       sync.Mutex
	version  string
	policy   SandboxPolicy
}

// NewSandboxRuntime configures the runtime.
func NewSandboxRuntime(config SandboxConfig) *SandboxRuntimeImpl {
	if config.RunscPath == "" {
		config.RunscPath = "runsc"
	}
	if config.Platform == "" {
		config.Platform = "kvm"
	}
	if config.ContainerRuntime == "" {
		config.ContainerRuntime = "docker"
	}
	if !config.NetworkIsolation {
		config.NetworkIsolation = true
	}
	return &SandboxRuntimeImpl{
		config: config,
	}
}

// Name implements SandboxRuntime.
func (g *SandboxRuntimeImpl) Name() string {
	return "gvisor"
}

// RunConfig returns the effective configuration.
func (g *SandboxRuntimeImpl) RunConfig() SandboxConfig {
	return g.config
}

// Capabilities reports the security properties the active backend can enforce.
func (g *SandboxRuntimeImpl) Capabilities() Capabilities {
	return Capabilities{
		NetworkIsolation:  true,
		ReadOnlyRoot:      true,
		ProtectedPaths:    true,
		NoNewPrivileges:   true,
		Seccomp:           true,
		UserMapping:       true,
		PerCommandWorkdir: true,
		EnvFiltering:      false,
	}
}

// ValidatePolicy checks policy structure and backend support before apply.
func (g *SandboxRuntimeImpl) ValidatePolicy(policy SandboxPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	caps := g.Capabilities()
	switch {
	case len(policy.AllowedEnvKeys) > 0 || len(policy.DeniedEnvKeys) > 0:
		if !caps.EnvFiltering {
			return fmt.Errorf("%s backend does not support environment filtering", g.Name())
		}
	}
	if policy.ReadOnlyRoot && !caps.ReadOnlyRoot {
		return fmt.Errorf("%s backend does not support read-only root", g.Name())
	}
	if len(policy.ProtectedPaths) > 0 && !caps.ProtectedPaths {
		return fmt.Errorf("%s backend does not support protected paths", g.Name())
	}
	if policy.NoNewPrivileges && !caps.NoNewPrivileges {
		return fmt.Errorf("%s backend does not support no-new-privileges", g.Name())
	}
	if strings.TrimSpace(policy.SeccompProfile) != "" && !caps.Seccomp {
		return fmt.Errorf("%s backend does not support seccomp profiles", g.Name())
	}
	return nil
}

// ApplyPolicy validates and stores the policy.
func (g *SandboxRuntimeImpl) ApplyPolicy(_ context.Context, policy SandboxPolicy) error {
	if err := g.ValidatePolicy(policy); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.policy = policy
	return nil
}

// Verify ensures runsc and the selected runtime are available.
func (g *SandboxRuntimeImpl) Verify(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.verified {
		return nil
	}
	if err := g.checkRunsc(ctx); err != nil {
		return err
	}
	if err := g.checkContainerRuntime(ctx); err != nil {
		return err
	}
	g.verified = true
	return nil
}

// Policy returns the currently enforced sandbox policy.
func (g *SandboxRuntimeImpl) Policy() SandboxPolicy {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.policy
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

// checkRunsc validates the runsc binary exists and matches the expected
// platform so we fail fast before attempting to launch sandboxes.
func (g *SandboxRuntimeImpl) checkRunsc(ctx context.Context) error {
	path, err := exec.LookPath(g.config.RunscPath)
	if err != nil {
		return fmt.Errorf("runsc binary not found: %w", err)
	}
	c, cancel := g.commandContext(ctx, path, "--version")
	defer cancel()
	output, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("runsc verification failed: %w", err)
	}
	g.version = strings.TrimSpace(string(output))
	if !strings.Contains(g.version, "runsc") {
		return errors.New("invalid runsc output")
	}
	if g.config.Platform != "" && !strings.Contains(strings.ToLower(g.version), g.config.Platform) {
		// Platform hint mismatch is logged via version string but no longer fatal so
		// installations that omit the platform label continue to work.
		g.version = fmt.Sprintf("%s (platform hint %s not found)", g.version, g.config.Platform)
	}
	return nil
}

// checkContainerRuntime ensures docker/containerd are installed and respond to
// a basic info command so the agent runtime can launch workloads later.
func (g *SandboxRuntimeImpl) checkContainerRuntime(ctx context.Context) error {
	runtime := strings.ToLower(g.config.ContainerRuntime)
	switch runtime {
	case "docker", "containerd":
	default:
		return fmt.Errorf("unsupported container runtime %s", g.config.ContainerRuntime)
	}
	_, err := exec.LookPath(runtime)
	if err != nil {
		return fmt.Errorf("%s binary not found: %w", runtime, err)
	}
	// We run a lightweight version command to ensure the selected container runtime is available.
	var args []string
	if runtime == "docker" {
		args = []string{"info", "--format", "'{{json .Runtimes}}'"}
	} else {
		args = []string{"--version"}
	}
	cmd, cancel := g.commandContext(ctx, runtime, args...)
	defer cancel()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s verification failed: %w", runtime, err)
	}
	return nil
}

// commandContext wraps exec.CommandContext with a consistent timeout to avoid
// hanging verification commands.
func (g *SandboxRuntimeImpl) commandContext(ctx context.Context, name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	return exec.CommandContext(ctx, name, args...), cancel
}
