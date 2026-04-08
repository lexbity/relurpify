//go:build !integration
// +build !integration

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type mockCommandRunner struct {
	stdout string
	stderr string
	err    error
	calls  []CommandRequest
}

func (m *mockCommandRunner) Run(ctx context.Context, req CommandRequest) (string, string, error) {
	m.calls = append(m.calls, req)
	if m.err != nil {
		return "", "", m.err
	}
	return m.stdout, m.stderr, nil
}

type mockCommandApprover struct {
	approve bool
	err     error
	calls   []commandApproveCall
}

type commandApproveCall struct {
	AgentID string
	RuleID  string
	Reason  string
	Command string
}

func (m *mockCommandApprover) ApproveCommand(ctx context.Context, agentID, ruleID, reason, command string) (bool, error) {
	m.calls = append(m.calls, commandApproveCall{
		AgentID: agentID,
		RuleID:  ruleID,
		Reason:  reason,
		Command: command,
	})
	return m.approve, m.err
}

func TestShellGuardBlockRule(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "should not run"}
	blacklist := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: block-dangerous-command
    pattern: "danger.*"
    reason: "Dangerous command detected"
    action: block`)

	guard := NewShellGuard(inner, blacklist, nil, "test-agent")
	_, _, err := guard.Run(context.Background(), CommandRequest{Args: []string{"bash", "-c", "dangerous command"}})
	if err == nil {
		t.Fatal("Run() should return an error for blocked commands")
	}
	if got := err.Error(); got != "shell filter blocked [block-dangerous-command]: Dangerous command detected" {
		t.Fatalf("unexpected error: %s", got)
	}
	if got := len(inner.calls); got != 0 {
		t.Fatalf("inner runner should not be called for blocked commands, got %d calls", got)
	}
}

func TestShellGuardHitlApproved(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "approved output"}
	manager := &mockCommandApprover{approve: true}
	blacklist := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: hitl-review-needed
    pattern: "review.*"
    reason: "Requires human approval"
    action: hitl`)

	guard := NewShellGuard(inner, blacklist, manager, "test-agent-123")
	stdout, _, err := guard.Run(context.Background(), CommandRequest{Args: []string{"bash", "-c", "review command"}})
	if err != nil {
		t.Fatalf("Run() should succeed for approved HITL command: %v", err)
	}
	if stdout != "approved output" {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
	if got := len(manager.calls); got != 1 {
		t.Fatalf("expected one approval call, got %d", got)
	}
	call := manager.calls[0]
	if call.AgentID != "test-agent-123" || call.RuleID != "hitl-review-needed" || call.Reason != "Requires human approval" || call.Command != "bash -c review command" {
		t.Fatalf("unexpected approval call: %+v", call)
	}
	if got := len(inner.calls); got != 1 {
		t.Fatalf("inner runner should be called exactly once, got %d", got)
	}
}

func TestShellGuardHitlDenied(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "should not run"}
	manager := &mockCommandApprover{approve: false}
	blacklist := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: hitl-deny-policy
    pattern: "deny.*"
    reason: "Human denied this command"
    action: hitl`)

	guard := NewShellGuard(inner, blacklist, manager, "test-agent-456")
	_, _, err := guard.Run(context.Background(), CommandRequest{Args: []string{"bash", "-c", "deny command"}})
	if err == nil {
		t.Fatal("Run() should return an error for denied HITL commands")
	}
	if got := err.Error(); got != "shell filter denied [hitl-deny-policy]: Human denied this command" {
		t.Fatalf("unexpected error: %s", got)
	}
	if got := len(manager.calls); got != 1 {
		t.Fatalf("expected one approval call, got %d", got)
	}
}

func TestShellGuardHitlError(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "should not run"}
	expectedErr := errors.New("approval service unavailable")
	manager := &mockCommandApprover{approve: true, err: expectedErr}
	blacklist := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: hitl-system-error
    pattern: "system.*"
    reason: "System check failed"
    action: hitl`)

	guard := NewShellGuard(inner, blacklist, manager, "test-agent-789")
	_, _, err := guard.Run(context.Background(), CommandRequest{Args: []string{"bash", "-c", "system check"}})
	if err == nil {
		t.Fatal("Run() should return an error when approval fails")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected approval error to be wrapped, got: %v", err)
	}
	if got := len(manager.calls); got != 1 {
		t.Fatalf("expected one approval call, got %d", got)
	}
}

func TestShellGuardHitlNoApprover(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "should not run"}
	blacklist := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: hitl-no-approver-configured
    pattern: "unapproved.*"
    reason: "Requires HITL approval but none configured"
    action: hitl`)

	guard := NewShellGuard(inner, blacklist, nil, "test-agent-none")
	_, _, err := guard.Run(context.Background(), CommandRequest{Args: []string{"bash", "-c", "unapproved command"}})
	if err == nil {
		t.Fatal("Run() should return an error when no approver is configured")
	}
	want := "shell filter requires HITL approval [hitl-no-approver-configured]: Requires HITL approval but none configured (no approver configured)"
	if got := err.Error(); got != want {
		t.Fatalf("unexpected error: %s", got)
	}
	if got := len(inner.calls); got != 0 {
		t.Fatalf("inner runner should not be called, got %d calls", got)
	}
}

func TestShellGuardNoMatchingRule(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "pass-through output"}
	manager := &mockCommandApprover{approve: true}
	blacklist := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: block-dangerous
    pattern: "danger.*"
    reason: "Dangerous command detected"
    action: block`)

	guard := NewShellGuard(inner, blacklist, manager, "test-agent-pass")
	stdout, _, err := guard.Run(context.Background(), CommandRequest{Args: []string{"echo", "hello"}})
	if err != nil {
		t.Fatalf("Run() should pass through for non-matching rules: %v", err)
	}
	if stdout != "pass-through output" {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
	if got := len(manager.calls); got != 0 {
		t.Fatalf("approver should not be called for non-matching commands, got %d calls", got)
	}
	if got := len(inner.calls); got != 1 {
		t.Fatalf("inner runner should be called exactly once, got %d", got)
	}
}

func TestShellGuardNilBlacklist(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "nil blacklist output"}
	manager := &mockCommandApprover{approve: true}
	guard := NewShellGuard(inner, nil, manager, "test-agent-nil")

	stdout, _, err := guard.Run(context.Background(), CommandRequest{Args: []string{"any", "command"}})
	if err != nil {
		t.Fatalf("Run() should succeed with a nil blacklist: %v", err)
	}
	if stdout != "nil blacklist output" {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
	if got := len(manager.calls); got != 0 {
		t.Fatalf("approver should not be called for nil blacklist, got %d calls", got)
	}
}

func TestShellGuardMultipleMatchingRules(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "pass-through"}
	manager := &mockCommandApprover{approve: true}
	blacklist := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: first-match-block
    pattern: "echo"
    reason: "Block all echo commands"
    action: block
  - id: second-match-hitl
    pattern: "echo.*hello"
    reason: "HITL for echo hello"
    action: hitl`)

	guard := NewShellGuard(inner, blacklist, manager, "test-agent-multi")
	_, _, err := guard.Run(context.Background(), CommandRequest{Args: []string{"echo", "hello"}})
	if err == nil {
		t.Fatal("Run() should fail on the first matching rule")
	}
	if got := err.Error(); got != "shell filter blocked [first-match-block]: Block all echo commands" {
		t.Fatalf("unexpected error: %s", got)
	}
	if got := len(manager.calls); got != 0 {
		t.Fatalf("approver should not be called when the first rule is block, got %d calls", got)
	}
	if got := len(inner.calls); got != 0 {
		t.Fatalf("inner runner should not be called for blocked commands, got %d calls", got)
	}
}

func TestShellGuardRequestStructure(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "inner-calls"}
	manager := &mockCommandApprover{approve: true}
	blacklist := mustLoadBlacklistFromYAML(t, `version: 1.0
rules:
  - id: exact-match-test
    pattern: "^echo hello$"
    reason: "Exact match required"
    action: block`)

	guard := NewShellGuard(inner, blacklist, manager, "test-agent-structure")
	_, _, err := guard.Run(context.Background(), CommandRequest{Args: []string{"echo", "hello"}})
	if err == nil {
		t.Fatal("Run() should fail for exact block rule")
	}
	if got := err.Error(); got != "shell filter blocked [exact-match-test]: Exact match required" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestNewShellGuard(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{}
	blacklist := &ShellBlacklist{}
	manager := &mockCommandApprover{}
	guard := NewShellGuard(inner, blacklist, manager, "agent")
	if guard.inner != inner || guard.blacklist != blacklist || guard.manager != manager || guard.agentID != "agent" {
		t.Fatalf("NewShellGuard did not preserve constructor arguments: %+v", guard)
	}
}

func TestShellGuardAllowsInnerErrorOnPassThrough(t *testing.T) {
	t.Parallel()

	expectedErr := fmt.Errorf("inner failure")
	inner := &mockCommandRunner{err: expectedErr}
	blacklist := &ShellBlacklist{}
	guard := NewShellGuard(inner, blacklist, nil, "agent")

	_, _, err := guard.Run(context.Background(), CommandRequest{Args: []string{"echo", "hello"}})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected inner error to be returned, got: %v", err)
	}
}
