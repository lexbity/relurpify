package agenttest

import (
	"context"
	"errors"
	"fmt"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
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
	sboxRuntime, err := fauthorization.SelectSandboxRuntime(backend, sandboxCfg, "", workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: select sandbox runtime: %w", ErrNoSandboxBackend, err)
	}
	policy := fauthorization.BuildSandboxPolicy(nil, nil)
	runner, err := sandbox.NewVerifiedCommandRunner(ctx, sboxRuntime, policy, &sandbox.CommandRunnerConfig{
		Workspace: workspaceRoot,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoSandboxBackend, err)
	}
	return runner, nil
}
