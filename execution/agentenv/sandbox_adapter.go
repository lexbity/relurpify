package agentenv

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
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

func (a *sandboxRuntimeAdapter) ValidatePolicy(p fauthorization.SandboxPolicy) error {
	return a.inner.ValidatePolicy(toSandboxPolicy(p))
}

func (a *sandboxRuntimeAdapter) ApplyPolicy(ctx context.Context, p fauthorization.SandboxPolicy) error {
	return a.inner.ApplyPolicy(ctx, toSandboxPolicy(p))
}

func (a *sandboxRuntimeAdapter) Policy() fauthorization.SandboxPolicy {
	return fromSandboxPolicy(a.inner.Policy())
}

func (a *sandboxRuntimeAdapter) RunConfig() fauthorization.SandboxConfig {
	return toFaSandboxConfig(a.inner.RunConfig())
}

func (a *sandboxRuntimeAdapter) Name() string {
	return a.inner.Name()
}

func toSandboxPolicy(p fauthorization.SandboxPolicy) sandbox.SandboxPolicy {
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

func fromSandboxPolicy(p sandbox.SandboxPolicy) fauthorization.SandboxPolicy {
	out := fauthorization.SandboxPolicy{
		ReadOnlyRoot:    p.ReadOnlyRoot,
		ProtectedPaths:  append([]string(nil), p.ProtectedPaths...),
		NoNewPrivileges: p.NoNewPrivileges,
		SeccompProfile:  p.SeccompProfile,
		AllowedEnvKeys:  append([]string(nil), p.AllowedEnvKeys...),
		DeniedEnvKeys:   append([]string(nil), p.DeniedEnvKeys...),
	}
	for _, r := range p.NetworkRules {
		out.NetworkRules = append(out.NetworkRules, fauthorization.NetworkRule{
			Direction:   r.Direction,
			Protocol:    r.Protocol,
			Host:        r.Host,
			Port:        r.Port,
			Description: r.Description,
		})
	}
	return out
}

// newSandboxBackendFactory returns a SandboxBackendFactory that creates
// sandbox runtimes using the capability/sandbox package.
func newSandboxBackendFactory() fauthorization.SandboxBackendFactory {
	return func(ctx context.Context, backend string, cfg fauthorization.SandboxConfig, image, workspace string) (fauthorization.SandboxRuntime, error) {
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

// reverseSandboxRuntimeAdapter adapts authorization.SandboxRuntime back to
// sandbox.SandboxRuntime so existing callers that pass registration.Runtime
// to sandbox.NewCommandRunner continue to work.
type reverseSandboxRuntimeAdapter struct {
	inner fauthorization.SandboxRuntime
}

func (a *reverseSandboxRuntimeAdapter) Verify(ctx context.Context) error {
	return a.inner.Verify(ctx)
}

func (a *reverseSandboxRuntimeAdapter) ValidatePolicy(p sandbox.SandboxPolicy) error {
	return a.inner.ValidatePolicy(fromSandboxPolicy(p))
}

func (a *reverseSandboxRuntimeAdapter) ApplyPolicy(ctx context.Context, p sandbox.SandboxPolicy) error {
	return a.inner.ApplyPolicy(ctx, fromSandboxPolicy(p))
}

func (a *reverseSandboxRuntimeAdapter) Policy() sandbox.SandboxPolicy {
	return toSandboxPolicy(a.inner.Policy())
}

func (a *reverseSandboxRuntimeAdapter) Capabilities() sandbox.Capabilities {
	return sandbox.Capabilities{}
}

func (a *reverseSandboxRuntimeAdapter) RunConfig() sandbox.SandboxConfig {
	rc := a.inner.RunConfig()
	return sandbox.SandboxConfig{
		RunscPath:        rc.RunscPath,
		ContainerRuntime: rc.ContainerRuntime,
		Platform:         rc.Platform,
		NetworkIsolation: rc.NetworkIsolation,
		ReadOnlyRoot:     rc.ReadOnlyRoot,
		SeccompProfile:   rc.SeccompProfile,
	}
}

func (a *reverseSandboxRuntimeAdapter) Name() string {
	return a.inner.Name()
}

// toFaSandboxConfig converts a sandbox.SandboxConfig to authorization.SandboxConfig.
func toFaSandboxConfig(cfg sandbox.SandboxConfig) fauthorization.SandboxConfig {
	return fauthorization.SandboxConfig{
		RunscPath:        cfg.RunscPath,
		ContainerRuntime: cfg.ContainerRuntime,
		Platform:         cfg.Platform,
		NetworkIsolation: cfg.NetworkIsolation,
		ReadOnlyRoot:     cfg.ReadOnlyRoot,
		SeccompProfile:   cfg.SeccompProfile,
	}
}
