package sandbox

import (
	"context"
	"errors"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// AuthorizedRunner is a CommandRunner that has been verified (sandbox boundary
// confirmed) AND wrapped with a CommandPolicy. It can only be constructed by
// NewAuthorizedRunner, which requires a non-nil policy. The unexported field
// prevents construction outside this package.
type AuthorizedRunner struct {
	inner     CommandRunner
	telemetry telemetry.Telemetry
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

// SetTelemetry attaches a framework telemetry sink so the sandbox chain emits
// command-forensics events (denied / executed / failure, FR-15). A nil sink is
// valid and keeps the runner silent. The same *AuthorizedRunner instance is
// shared by every caller in the invariant chain, so mutating it here reaches
// all of them.
func (a *AuthorizedRunner) SetTelemetry(tel telemetry.Telemetry) *AuthorizedRunner {
	if a != nil {
		a.telemetry = tel
	}
	return a
}

// Run applies the command policy before delegating to the underlying verified
// and enforced runner.
func (a *AuthorizedRunner) Run(ctx context.Context, req CommandRequest) (*ports.CommandResult, error) {
	if a == nil || a.inner == nil {
		return nil, errors.New("authorized runner missing")
	}
	started := time.Now().UTC()
	res, err := a.inner.Run(ctx, req)
	a.emitOutcome(ctx, req, res, err, started)
	return res, err
}

// Compile-time guarantees.
var _ CommandRunner = (*AuthorizedRunner)(nil)
