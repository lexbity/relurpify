package sandbox

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnforcingCommandRunner_DeniesBeforeInnerRunner(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "should not run"}
	runner := NewEnforcingCommandRunner(inner, CommandPolicyFunc(func(context.Context, CommandRequest) error {
		return errors.New("blocked by policy")
	}))

	_, _, err := runner.Run(context.Background(), CommandRequest{Args: []string{"echo", "hello"}})
	require.Error(t, err)

	var denied *ExecutionDeniedError
	require.ErrorAs(t, err, &denied)
	require.Equal(t, "echo hello", denied.Command)
	require.Contains(t, denied.Error(), "execution denied")
	require.Len(t, inner.calls, 0)
}

func TestEnforcingCommandRunner_AllowsInnerRunnerWhenPolicyPasses(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "ok"}
	runner := NewEnforcingCommandRunner(inner, CommandPolicyFunc(func(context.Context, CommandRequest) error {
		return nil
	}))

	stdout, stderr, err := runner.Run(context.Background(), CommandRequest{Args: []string{"echo", "hello"}})
	require.NoError(t, err)
	require.Equal(t, "ok", stdout)
	require.Empty(t, stderr)
	require.Len(t, inner.calls, 1)
}

func TestEnforcingCommandRunner_ComposesWithShellGuardPolicy(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "should not run"}
	blacklist := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: block-touch
    pattern: "touch .*"
    reason: "filesystem mutation blocked"
    action: block`)
	guard := NewShellGuard(nil, blacklist, nil, "agent-1")
	runner := NewEnforcingCommandRunner(inner, guard)

	_, _, err := runner.Run(context.Background(), CommandRequest{Args: []string{"touch", "file.txt"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "shell filter blocked [block-touch]")
	require.Len(t, inner.calls, 0)
}
