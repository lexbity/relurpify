package dockersandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/sandbox"
)

// Config controls the Docker sandbox backend.
type Config struct {
	DockerPath string
	Image      string
	Workspace  string
}

// Backend implements the backend-neutral sandbox policy contract using Docker.
type Backend struct {
	mu       sync.Mutex
	config   Config
	verified bool
	policy   sandbox.SandboxPolicy
}

// NewBackend constructs a Docker sandbox backend.
func NewBackend(cfg Config) *Backend {
	if cfg.DockerPath == "" {
		cfg.DockerPath = "docker"
	}
	if cfg.Image == "" {
		cfg.Image = "ghcr.io/relurpify/runtime:latest"
	}
	if abs, err := filepath.Abs(cfg.Workspace); err == nil && abs != "" {
		cfg.Workspace = filepath.Clean(abs)
	}
	return &Backend{config: cfg}
}

// Name identifies the backend.
func (b *Backend) Name() string {
	return "docker"
}

// Capabilities reports the security features this backend can enforce.
func (b *Backend) Capabilities() sandbox.Capabilities {
	return sandbox.Capabilities{
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

// Verify checks that Docker is installed and reachable.
func (b *Backend) Verify(ctx context.Context) error {
	if b == nil {
		return errors.New("docker backend missing")
	}
	b.mu.Lock()
	if b.verified {
		b.mu.Unlock()
		return nil
	}
	dockerPath := b.config.DockerPath
	b.mu.Unlock()

	path, err := exec.LookPath(dockerPath)
	if err != nil {
		return fmt.Errorf("docker binary not found: %w", err)
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, path, "version", "--format", "{{.Server.Version}}")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker verification failed: %w", err)
	} else if strings.TrimSpace(string(out)) == "" {
		return errors.New("docker verification returned empty version output")
	}

	b.mu.Lock()
	b.verified = true
	b.mu.Unlock()
	return nil
}

// ValidatePolicy rejects policy fields this backend cannot safely enforce.
func (b *Backend) ValidatePolicy(policy sandbox.SandboxPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	caps := b.Capabilities()
	if (len(policy.AllowedEnvKeys) > 0 || len(policy.DeniedEnvKeys) > 0) && !caps.EnvFiltering {
		return fmt.Errorf("%s backend does not support environment filtering", b.Name())
	}
	if len(policy.NetworkRules) > 0 {
		return fmt.Errorf("%s backend does not support granular network rules", b.Name())
	}
	if len(policy.ProtectedPaths) > 0 {
		if strings.TrimSpace(b.config.Workspace) == "" {
			return fmt.Errorf("%s backend requires a workspace to enforce protected paths", b.Name())
		}
		for _, path := range policy.ProtectedPaths {
			if err := b.validateProtectedPath(path); err != nil {
				return err
			}
		}
	}
	return nil
}

// ApplyPolicy validates and stores the active policy.
func (b *Backend) ApplyPolicy(_ context.Context, policy sandbox.SandboxPolicy) error {
	if err := b.ValidatePolicy(policy); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.policy = clonePolicy(policy)
	return nil
}

// Policy returns the active policy snapshot.
func (b *Backend) Policy() sandbox.SandboxPolicy {
	if b == nil {
		return sandbox.SandboxPolicy{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return clonePolicy(b.policy)
}

// RunConfig exposes the container runtime settings expected by callers that
// still request a sandbox runtime configuration.
func (b *Backend) RunConfig() sandbox.SandboxConfig {
	if b == nil {
		return sandbox.SandboxConfig{}
	}
	return sandbox.SandboxConfig{
		ContainerRuntime: b.config.DockerPath,
		NetworkIsolation: true,
		ReadOnlyRoot:     true,
	}
}

// NewCommandRunner builds the Docker-specific command runner used by the
// framework when this backend is selected.
func (b *Backend) NewCommandRunner(_ *manifest.AgentManifest, workspace string) (sandbox.CommandRunner, error) {
	clone := *b
	clone.config.Workspace = workspace
	return NewRunner(&clone)
}

func (b *Backend) validateProtectedPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("protected path required")
	}
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return fmt.Errorf("%s backend requires protected paths to be absolute: %s", b.Name(), path)
	}
	if _, err := os.Stat(clean); err != nil {
		return fmt.Errorf("%s backend requires protected path to exist: %s: %w", b.Name(), clean, err)
	}
	workspace := filepath.Clean(b.config.Workspace)
	if workspace == "." || workspace == "" {
		return fmt.Errorf("%s backend requires absolute workspace for protected paths", b.Name())
	}
	rel, err := filepath.Rel(workspace, clean)
	if err != nil {
		return fmt.Errorf("%s backend could not resolve protected path %s: %w", b.Name(), clean, err)
	}
	if strings.HasPrefix(rel, "..") {
		return fmt.Errorf("%s backend requires protected path inside workspace: %s", b.Name(), clean)
	}
	return nil
}

func clonePolicy(policy sandbox.SandboxPolicy) sandbox.SandboxPolicy {
	out := sandbox.SandboxPolicy{
		NetworkRules:    append([]sandbox.NetworkRule(nil), policy.NetworkRules...),
		ReadOnlyRoot:    policy.ReadOnlyRoot,
		ProtectedPaths:  append([]string(nil), policy.ProtectedPaths...),
		NoNewPrivileges: policy.NoNewPrivileges,
		SeccompProfile:  policy.SeccompProfile,
		AllowedEnvKeys:  append([]string(nil), policy.AllowedEnvKeys...),
		DeniedEnvKeys:   append([]string(nil), policy.DeniedEnvKeys...),
	}
	return out
}

var _ sandbox.Backend = (*Backend)(nil)
