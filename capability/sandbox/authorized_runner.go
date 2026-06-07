package sandbox

import (
	"context"
	"errors"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// AuthorizedRunner is a CommandRunner that has been verified (sandbox boundary
// confirmed) AND wrapped with a CommandPolicy. It can only be constructed by
// NewAuthorizedRunner, which requires a non-nil policy. The unexported field
// prevents construction outside this package.
type AuthorizedRunner struct {
	inner CommandRunner
}

// NewAuthorizedRunner wraps a verified runner with a command policy check.
// Returns an error if policy is nil — an authorized runner must always have
// a policy. This is the only way to construct an AuthorizedRunner.
func NewAuthorizedRunner(verified CommandRunner, policy CommandPolicy) (*AuthorizedRunner, error) {
	if policy == nil {
		return nil, errors.New("authorized runner requires a non-nil command policy")
	}
	return &AuthorizedRunner{
		inner: NewEnforcingCommandRunner(verified, policy),
	}, nil
}

// Run applies the command policy before delegating to the underlying verified
// and enforced runner.
func (a *AuthorizedRunner) Run(ctx context.Context, req CommandRequest) (*ports.CommandResult, error) {
	if a == nil || a.inner == nil {
		return nil, errors.New("authorized runner missing")
	}
	return a.inner.Run(ctx, req)
}

// Compile-time guarantees.
var _ CommandRunner = (*AuthorizedRunner)(nil)
