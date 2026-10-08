package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/capability/sandbox/safeexec"
	"codeburg.org/lexbit/relurpify/governance/netpolicy"
)

// Validate/Supports methods live on the sandbox types.

// SandboxRuntimeImpl enforces runsc-backed execution.
type SandboxRuntimeImpl struct {
	config   SandboxConfig
	verified bool
	mu       sync.Mutex
	version  string
	policy   SandboxPolicy
}

// Compile-time guarantee that the gVisor runtime satisfies the sandbox API.
var _ SandboxRuntime = (*SandboxRuntimeImpl)(nil)

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
	return validateBackendPolicy(g.Name(), g.Capabilities(), policy)
}

// validateBackendPolicy applies the shared, backend-neutral policy checks every
// sandbox backend must enforce. Backends supply their name and enforced
// capabilities; a policy rejected by one backend for a structural reason is
// rejected by all of them.
func validateBackendPolicy(name string, caps Capabilities, policy SandboxPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	// Mandatory egress denylist: declared network rules must not target a
	// non-public IP literal. Hostnames are not resolved here (no I/O at policy
	// validation); they are classified per-invocation by the egress scanner.
	// This makes network egress a sandbox-owned security boundary (SSRF and
	// cloud metadata protection) that no agent configuration can relax.
	for i, rule := range policy.NetworkRules {
		if !strings.EqualFold(strings.TrimSpace(rule.Direction), "egress") {
			continue // the denylist governs egress dial targets, not bind addresses
		}
		if class, literal := netpolicy.ClassifyToken(rule.Host); literal && class != netpolicy.ClassPublic {
			return fmt.Errorf("%s backend: network rule %d targets blocked host %q (%s addresses are denied)", name, i, rule.Host, class)
		}
	}
	switch {
	case len(policy.AllowedEnvKeys) > 0 || len(policy.DeniedEnvKeys) > 0:
		if !caps.EnvFiltering {
			return fmt.Errorf("%s backend does not support environment filtering", name)
		}
	}
	if policy.ReadOnlyRoot && !caps.ReadOnlyRoot {
		return fmt.Errorf("%s backend does not support read-only root", name)
	}
	if len(policy.ProtectedPaths) > 0 && !caps.ProtectedPaths {
		return fmt.Errorf("%s backend does not support protected paths", name)
	}
	if policy.NoNewPrivileges && !caps.NoNewPrivileges {
		return fmt.Errorf("%s backend does not support no-new-privileges", name)
	}
	if strings.TrimSpace(policy.SeccompProfile) != "" && !caps.Seccomp {
		return fmt.Errorf("%s backend does not support seccomp profiles", name)
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
	return verifyContainerRuntime(ctx, g.config.ContainerRuntime)
}

// verifyContainerRuntime checks that the named container runtime binary exists
// and responds to a lightweight command. It is shared by every sandbox backend
// so docker/containerd availability is probed identically.
func verifyContainerRuntime(ctx context.Context, runtime string) error {
	runtime = strings.ToLower(strings.TrimSpace(runtime))
	switch runtime {
	case "docker", "containerd":
	default:
		return fmt.Errorf("unsupported container runtime %s", runtime)
	}
	path, err := exec.LookPath(runtime)
	if err != nil {
		return fmt.Errorf("%s binary not found: %w", runtime, err)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var args []string
	if runtime == "docker" {
		args = []string{"info", "--format", "'{{json .Runtimes}}'"}
	} else {
		args = []string{"--version"}
	}
	cmd := safeexec.CommandContext(ctx, safeexec.Prepare(path, args...))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s verification failed: %w", runtime, err)
	}
	return nil
}

// commandContext wraps exec.CommandContext with a consistent timeout to avoid
// hanging verification commands.
func (g *SandboxRuntimeImpl) commandContext(ctx context.Context, name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	resolvedPath, err := exec.LookPath(name)
	if err != nil {
		resolvedPath = name
	}
	cmd := safeexec.CommandContext(ctx, safeexec.Prepare(resolvedPath, args...))
	return cmd, cancel
}

// SupportedSandboxBackends returns the canonical list of valid sandbox backend
// names. This is the single source of truth for which backends exist; both
// authorization.SelectSandboxRuntime and cfgload validation derive from it.
func SupportedSandboxBackends() []string {
	return []string{"gvisor", "docker"}
}

// IsSupportedSandboxBackend returns true if the given backend name is a valid
// sandbox backend. It does NOT treat "" as supported — the caller (typically
// SelectSandboxRuntime) handles the empty/default case separately.
func IsSupportedSandboxBackend(backend string) bool {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "gvisor", "docker":
		return true
	default:
		return false
	}
}
