package envcomposition

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
)

// sandboxRuntimeAdapter adapts sandbox.SandboxRuntime to the governance
// authorization.SandboxRuntime interface so callers can create sandbox
// runtimes without governance importing the sandbox package.
type sandboxRuntimeAdapter struct {
	inner sandbox.SandboxRuntime
}

func (a *sandboxRuntimeAdapter) Verify(ctx context.Context) error {
	return a.inner.Verify(ctx)
}

func (a *sandboxRuntimeAdapter) ValidatePolicy(p governanceports.SandboxPolicy) error {
	return a.inner.ValidatePolicy(toSandboxPolicy(p))
}

func (a *sandboxRuntimeAdapter) ApplyPolicy(ctx context.Context, p governanceports.SandboxPolicy) error {
	return a.inner.ApplyPolicy(ctx, toSandboxPolicy(p))
}

func (a *sandboxRuntimeAdapter) Policy() governanceports.SandboxPolicy {
	return fromSandboxPolicy(a.inner.Policy())
}

func (a *sandboxRuntimeAdapter) RunConfig() governanceports.SandboxConfig {
	return toFaSandboxConfig(a.inner.RunConfig())
}

func (a *sandboxRuntimeAdapter) Name() string {
	return a.inner.Name()
}

func toSandboxPolicy(p governanceports.SandboxPolicy) sandbox.SandboxPolicy {
	out := sandbox.SandboxPolicy{
		ReadOnlyRoot:    p.ReadOnlyRoot,
		ProtectedPaths:  append([]string(nil), p.ProtectedPaths...),
		NoNewPrivileges: p.NoNewPrivileges,
		SeccompProfile:  p.SeccompProfile,
		AllowedEnvKeys:  append([]string(nil), p.AllowedEnvKeys...),
		DeniedEnvKeys:   append([]string(nil), p.DeniedEnvKeys...),
	}
	for _, r := range p.NetworkRules {
		out.NetworkRules = append(out.NetworkRules, sandbox.NetworkRule{
			Direction:   r.Direction,
			Protocol:    r.Protocol,
			Host:        r.Host,
			Port:        r.Port,
			Description: r.Description,
		})
	}
	return out
}

func fromSandboxPolicy(p sandbox.SandboxPolicy) governanceports.SandboxPolicy {
	out := governanceports.SandboxPolicy{
		ReadOnlyRoot:    p.ReadOnlyRoot,
		ProtectedPaths:  append([]string(nil), p.ProtectedPaths...),
		NoNewPrivileges: p.NoNewPrivileges,
		SeccompProfile:  p.SeccompProfile,
		AllowedEnvKeys:  append([]string(nil), p.AllowedEnvKeys...),
		DeniedEnvKeys:   append([]string(nil), p.DeniedEnvKeys...),
	}
	for _, r := range p.NetworkRules {
		out.NetworkRules = append(out.NetworkRules, governanceports.SandboxNetworkRule{
			Direction:   r.Direction,
			Protocol:    r.Protocol,
			Host:        r.Host,
			Port:        r.Port,
			Description: r.Description,
		})
	}
	return out
}

// NewSandboxBackendFactory returns a SandboxBackendFactory that creates
// sandbox runtimes using the capability/sandbox package.
func NewSandboxBackendFactory() fauthorization.SandboxBackendFactory {
	return func(ctx context.Context, backend string, cfg governanceports.SandboxConfig, image, workspace string) (governanceports.SandboxRuntime, error) {
		b := strings.ToLower(strings.TrimSpace(backend))
		if b == "" {
			b = "gvisor"
		}
		if !sandbox.IsSupportedSandboxBackend(b) {
			supported := strings.Join(sandbox.SupportedSandboxBackends(), ", ")
			return nil, fmt.Errorf("unsupported sandbox backend %q (supported: %s)", backend, supported)
		}
		switch b {
		case "gvisor":
			return &sandboxRuntimeAdapter{
				inner: sandbox.NewSandboxRuntime(sandbox.SandboxConfig{
					RunscPath:        cfg.RunscPath,
					ContainerRuntime: cfg.ContainerRuntime,
					Platform:         cfg.Platform,
					NetworkIsolation: cfg.NetworkIsolation,
					ReadOnlyRoot:     cfg.ReadOnlyRoot,
					SeccompProfile:   cfg.SeccompProfile,
				}),
			}, nil
		default:
			return nil, fmt.Errorf("unreachable: unsupported sandbox backend %q", b)
		}
	}
}

// toFaSandboxConfig converts a sandbox.SandboxConfig to authorization.SandboxConfig.
func toFaSandboxConfig(cfg sandbox.SandboxConfig) governanceports.SandboxConfig {
	return governanceports.SandboxConfig{
		RunscPath:        cfg.RunscPath,
		ContainerRuntime: cfg.ContainerRuntime,
		Platform:         cfg.Platform,
		NetworkIsolation: cfg.NetworkIsolation,
		ReadOnlyRoot:     cfg.ReadOnlyRoot,
		SeccompProfile:   cfg.SeccompProfile,
	}
}
