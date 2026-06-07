package sandbox

import (
	"context"
	"errors"
	"fmt"
)

// NewVerifiedCommandRunner verifies the runtime, applies the policy, and returns a
// sandbox-backed CommandRunner.
func NewVerifiedCommandRunner(ctx context.Context, runtime SandboxRuntime, policy SandboxPolicy, config *CommandRunnerConfig) (CommandRunner, error) {
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
