package sandbox

import (
	"context"
	"errors"
	"fmt"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// NewVerifiedCommandRunner verifies the runtime, applies the policy, and returns a
// sandbox-backed CommandRunner. It NEVER returns a host-exec runner. A nil or
// unverifiable runtime is a hard error (fail-closed).
//
// Steps:
//  1. nil-check runtime
//  2. runtime.Verify(ctx) — ensures the sandbox backend is operational
//  3. runtime.ValidatePolicy(policy) — rejects policy fields the backend can't enforce
//  4. runtime.ApplyPolicy(ctx, policy) — stores the active policy
//  5. NewCommandRunner(config, runtime) — constructs the runner
//
// Verify is idempotent (backends cache the "verified" state), so calling it
// here and in RegisterAgent is safe.
func NewVerifiedCommandRunner(ctx context.Context, runtime SandboxRuntime, policy SandboxPolicy, config *contracts.CommandRunnerConfig) (CommandRunner, error) {
	if runtime == nil {
		return nil, errors.New("sandbox runtime required")
	}
	if err := runtime.Verify(ctx); err != nil {
		return nil, fmt.Errorf("sandbox verification failed: %w", err)
	}
	if err := runtime.ValidatePolicy(policy); err != nil {
		return nil, fmt.Errorf("sandbox policy validation failed: %w", err)
	}
	if err := runtime.ApplyPolicy(ctx, policy); err != nil {
		return nil, fmt.Errorf("sandbox policy application failed: %w", err)
	}
	return NewCommandRunner(config, runtime)
}
