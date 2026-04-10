package sandbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// CommandPolicy decides whether a command request may proceed.
type CommandPolicy interface {
	AllowCommand(ctx context.Context, req CommandRequest) error
}

// CommandPolicyFunc adapts a function to CommandPolicy.
type CommandPolicyFunc func(context.Context, CommandRequest) error

// AllowCommand implements CommandPolicy.
func (f CommandPolicyFunc) AllowCommand(ctx context.Context, req CommandRequest) error {
	if f == nil {
		return nil
	}
	return f(ctx, req)
}

// ExecutionDeniedError reports that sandbox enforcement blocked execution
// before the underlying runner was invoked.
type ExecutionDeniedError struct {
	Command string
	Reason  string
	Policy  string
	Cause   error
}

// Error implements error.
func (e *ExecutionDeniedError) Error() string {
	if e == nil {
		return ""
	}
	command := strings.TrimSpace(e.Command)
	reason := strings.TrimSpace(e.Reason)
	if e.Policy != "" {
		if command == "" {
			return fmt.Sprintf("execution denied by %s: %s", e.Policy, reason)
		}
		return fmt.Sprintf("execution denied by %s: %s (%s)", e.Policy, command, reason)
	}
	if command == "" {
		return fmt.Sprintf("execution denied: %s", reason)
	}
	return fmt.Sprintf("execution denied: %s (%s)", command, reason)
}

// Unwrap exposes the underlying policy error.
func (e *ExecutionDeniedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// EnforcingCommandRunner ensures a policy decision is applied before the
// underlying runner receives the command request.
type EnforcingCommandRunner struct {
	inner  CommandRunner
	policy CommandPolicy
}

// NewEnforcingCommandRunner wraps a runner with policy enforcement.
func NewEnforcingCommandRunner(inner CommandRunner, policy CommandPolicy) *EnforcingCommandRunner {
	return &EnforcingCommandRunner{
		inner:  inner,
		policy: policy,
	}
}

// Run applies the command policy before delegating to the underlying runner.
func (r *EnforcingCommandRunner) Run(ctx context.Context, req CommandRequest) (string, string, error) {
	if r == nil || r.inner == nil {
		return "", "", errors.New("enforcing command runner missing")
	}
	if r.policy != nil {
		if err := r.policy.AllowCommand(ctx, req); err != nil {
			return "", "", &ExecutionDeniedError{
				Command: strings.Join(req.Args, " "),
				Reason:  err.Error(),
				Policy:  "sandbox policy",
				Cause:   err,
			}
		}
	}
	return r.inner.Run(ctx, req)
}

// Ensure EnforcingCommandRunner satisfies the interface.
var _ CommandRunner = (*EnforcingCommandRunner)(nil)
