package agenttest

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
)

// ErrNoSandboxBackend is returned by NewWorkspaceSandboxRunner when the
// configured sandbox backend is unavailable (e.g. runsc/docker not installed).
// Callers can use errors.Is to detect this and skip tests.
var ErrNoSandboxBackend = errors.New("no sandbox backend available")

// NewWorkspaceSandboxRunner constructs a verified, sandbox-backed CommandRunner
// scoped to workspaceRoot. Each call creates an independent runner; the
// underlying sandbox (gVisor/Docker) provides per-command isolation.
//
// If the sandbox backend cannot be selected or verified, the returned error
// wraps ErrNoSandboxBackend so callers can skip tests with RequireSandbox.
func NewWorkspaceSandboxRunner(ctx context.Context, workspaceRoot, backend string) (sandbox.CommandRunner, error) {
	sandboxCfg := sandbox.SandboxConfig{}
	sboxRuntime, err := newSandboxRuntime(backend, sandboxCfg, "", workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: select sandbox runtime: %w", ErrNoSandboxBackend, err)
	}
	policy := sandbox.SandboxPolicy{
		ProtectedPaths: nil,
	}
	runner, err := sandbox.NewVerifiedCommandRunner(ctx, sboxRuntime, policy, &sandbox.CommandRunnerConfig{
		Workspace: workspaceRoot,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoSandboxBackend, err)
	}
	return runner, nil
}

func newSandboxRuntime(backend string, sandboxCfg sandbox.SandboxConfig, image, workspace string) (sandbox.SandboxRuntime, error) {
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
		return sandbox.NewSandboxRuntime(sandboxCfg), nil
	default:
		return nil, fmt.Errorf("unreachable: unsupported sandbox backend %q", b)
	}
}
