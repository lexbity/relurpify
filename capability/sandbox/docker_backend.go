package sandbox

import (
	"context"
	"sync"
)

// DockerSandboxBackend implements SandboxRuntime using Docker's native security
// features (seccomp, dropped capabilities, network isolation, read-only root)
// instead of the gVisor runsc runtime. It is selected with the "docker" backend
// name and is the alternative for hosts where runsc is unavailable.
//
// Security posture: container-level isolation. Docker's default AppArmor
// profile plus the configured seccomp profile confine syscalls, every Linux
// capability is dropped, the root filesystem is read-only (with a writable
// /tmp tmpfs), and the network is disabled unless NetworkIsolation is turned
// off. This is less robust than gVisor's userspace kernel but sufficient for
// many workloads.
type DockerSandboxBackend struct {
	config   SandboxConfig
	verified bool
	mu       sync.Mutex
	policy   SandboxPolicy
}

// Compile-time guarantees that the docker backend satisfies the sandbox API and
// can supply its own command runner.
var (
	_ SandboxRuntime        = (*DockerSandboxBackend)(nil)
	_ CommandRunnerProvider = (*DockerSandboxBackend)(nil)
)

// NewDockerSandboxBackend configures a Docker-native sandbox runtime. Network
// isolation defaults to enabled; callers must opt out explicitly.
func NewDockerSandboxBackend(config SandboxConfig) *DockerSandboxBackend {
	if config.ContainerRuntime == "" {
		config.ContainerRuntime = "docker"
	}
	if !config.NetworkIsolation {
		config.NetworkIsolation = true
	}
	return &DockerSandboxBackend{config: config}
}

// Name implements SandboxRuntime.
func (d *DockerSandboxBackend) Name() string { return "docker" }

// RunConfig returns the effective configuration.
func (d *DockerSandboxBackend) RunConfig() SandboxConfig { return d.config }

// Capabilities reports the security properties Docker native isolation enforces.
func (d *DockerSandboxBackend) Capabilities() Capabilities {
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
func (d *DockerSandboxBackend) ValidatePolicy(policy SandboxPolicy) error {
	return validateBackendPolicy(d.Name(), d.Capabilities(), policy)
}

// ApplyPolicy validates and stores the policy.
func (d *DockerSandboxBackend) ApplyPolicy(_ context.Context, policy SandboxPolicy) error {
	if err := d.ValidatePolicy(policy); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.policy = policy
	return nil
}

// Policy returns the currently enforced sandbox policy.
func (d *DockerSandboxBackend) Policy() SandboxPolicy {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.policy
}

// Verify ensures the configured container runtime is available.
func (d *DockerSandboxBackend) Verify(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.verified {
		return nil
	}
	if err := verifyContainerRuntime(ctx, d.config.ContainerRuntime); err != nil {
		return err
	}
	d.verified = true
	return nil
}

// NewCommandRunner supplies a Docker-native command runner. It omits
// `--runtime runsc` and adds `--cap-drop ALL` and a writable /tmp tmpfs so the
// container runs under Docker's own isolation boundary.
func (d *DockerSandboxBackend) NewCommandRunner(config *CommandRunnerConfig) (CommandRunner, error) {
	runner, err := newSandboxCommandRunner(config, d, true)
	if err != nil {
		return nil, err
	}
	return runner, nil
}
