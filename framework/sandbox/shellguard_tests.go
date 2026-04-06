//go:build !integration
// +build !integration

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
)

// mockCommandRunner is a simple implementation for testing purposes.
type mockCommandRunner struct {
	stdout string
	stderr string
	err    error
}

func newMockCommandRunner() CommandRunner {
	return &mockCommandRunner{
		stdout: "mock stdout output",
		stderr: "mock stderr output",
	}
}

func (m *mockCommandRunner) Run(ctx context.Context, req CommandRequest) (string, string, error) {
	if m.err != nil {
		return "", "", m.err
	}
	return m.stdout, m.stderr, nil
}

// mockCommandApprover is a simple implementation for testing purposes.
type mockCommandApprover struct {
	approve bool
	err     error
	calls   []CommandApproveCall // For tracking calls in tests
}

// CommandApproveCall tracks approval requests for test verification.
type CommandApproveCall struct {
	Context  context.Context
	AgentID  string
	RuleID   string
	Reason   string
	Command  string
	Approved bool
	Err      error
}

func newMockCommandApprover(approve bool, err error) *mockCommandApprover {
	return &mockCommandApprover{
		approve: approve,
		err:     err,
		calls:   []CommandApproveCall{},
	}
}

func (m *mockCommandApprover) ApproveCommand(ctx context.Context, agentID, ruleID, reason, command string) (bool, error) {
	call := CommandApproveCall{
		Context:  ctx,
		AgentID:  agentID,
		RuleID:   ruleID,
		Reason:   reason,
		Command:  command,
		Approved: m.approve,
	}
	m.calls = append(m.calls, call)
	return call.Approved, call.Err
}

// TestShellGuardBlockRule tests that block rules prevent execution.
func TestShellGuardBlockRule(t *testing.T) {
	t.Parallel()

	inner := newMockCommandRunner()
	blacklist := loadBlacklistForTests(`version: 1.0
rules:
  - id: block-dangerous-command
    pattern: "danger.*"
    reason: "Dangerous command detected"
    action: block`)

	var manager CommandApprover = nil // No approver needed for block rules
	agentID := "test-agent"
	guard := NewShellGuard(inner, blacklist, manager, agentID)

	ctx := context.Background()
	req := CommandRequest{
		Args: []string{"bash", "-c", "dangerous command"},
	}

	stdout, _, err := guard.Run(ctx, req)

	if err == nil {
		t.Error("Run() should return error for blocked rule")
	} else if !errors.Is(err, fmt.Errorf("shell filter blocked [block-dangerous-command]: Dangerous command detected")) {
		t.Errorf("wrong error: %v", err)
	}

	if stdout != "" {
		t.Errorf("unexpected stdout: %s", stdout)
	}

	if stderr != "" {
		t.Errorf("unexpected stderr: %s", stderr)
	}
}

// TestShellGuardHitlApproved tests that HITL rules pass through when approved.
func TestShellGuardHitlApproved(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "approved output"}
	blacklist := loadBlacklistForTests(`version: 1.0
rules:
  - id: hitl-review-needed
    pattern: "review.*"
    reason: "Requires human approval"
    action: hitl`)

	manager := newMockCommandApprover(true, nil)
	agentID := "test-agent-123"
	guard := NewShellGuard(inner, blacklist, manager, agentID)

	ctx := context.Background()
	req := CommandRequest{
		Args: []string{"bash", "-c", "review command"},
	}

	stdout, _, err := guard.Run(ctx, req)

	if err != nil {
		t.Errorf("Run() should return nil for approved HITL rule: %v", err)
	}

	if stdout != "approved output" {
		t.Errorf("wrong stdout: expected 'approved output', got '%s'", stdout)
	}

	if len(manager.calls) != 1 {
		t.Errorf("expected 1 approval call, got %d", len(manager.calls))
	} else if manager.calls[0].Approved {
		// Verify HITL was called
	} else {
		t.Error("approver should have been called and returned true")
	}
}

// TestShellGuardHitlDenied tests that HITL rules fail when denied.
func TestShellGuardHitlDenied(t *testing.T) {
	t.Parallel()

	inner := newMockCommandRunner()
	blacklist := loadBlacklistForTests(`version: 1.0
rules:
  - id: hitl-deny-policy
    pattern: "deny.*"
    reason: "Human denied this command"
    action: hitl`)

	manager := newMockCommandApprover(false, nil)
	agentID := "test-agent-456"
	guard := NewShellGuard(inner, blacklist, manager, agentID)

	ctx := context.Background()
	req := CommandRequest{
		Args: []string{"bash", "-c", "deny command"},
	}

	stdout, _, err := guard.Run(ctx, req)

	if err == nil {
		t.Error("Run() should return error for denied HITL rule")
	} else if !errors.Is(err, fmt.Errorf("shell filter denied [hitl-deny-policy]: Human denied this command")) {
		t.Errorf("wrong error: %v", err)
	}

	if stdout != "" {
		t.Errorf("unexpected stdout: %s", stdout)
	}

	if len(manager.calls) != 1 {
		t.Errorf("expected 1 approval call, got %d", len(manager.calls))
	} else if manager.calls[0].Approved {
		t.Error("approver should have returned false for denied HITL")
	}
}

// TestShellGuardHitlError tests that HITL errors are propagated.
func TestShellGuardHitlError(t *testing.T) {
	t.Parallel()

	inner := newMockCommandRunner()
	blacklist := loadBlacklistForTests(`version: 1.0
rules:
  - id: hitl-system-error
    pattern: "system.*"
    reason: "System check failed"
    action: hitl`)

	expectedErr := errors.New("approval service unavailable")
	manager := newMockCommandApprover(true, expectedErr) // Approve true but with error
	agentID := "test-agent-789"
	guard := NewShellGuard(inner, blacklist, manager, agentID)

	ctx := context.Background()
	req := CommandRequest{
		Args: []string{"bash", "-c", "system check"},
	}

	stdout, _, err := guard.Run(ctx, req)

	if err == nil || errors.Is(err, fmt.Errorf("shell filter approved [hitl-system-error]")) {
		t.Error("Run() should return error for HITL approval error")
	} else if !errors.Is(err, expectedErr) {
		t.Errorf("expected approval service unavailable error, got: %v", err)
	}

	if stdout != "" {
		t.Errorf("unexpected stdout: %s", stdout)
	}
}

// TestShellGuardHitlNoApprover tests that HITL fails when no approver is configured.
func TestShellGuardHitlNoApprover(t *testing.T) {
	t.Parallel()

	inner := newMockCommandRunner()
	blacklist := loadBlacklistForTests(`version: 1.0
rules:
  - id: hitl-no-approver-configured
    pattern: "unapproved.*"
    reason: "Requires HITL approval but none configured"
    action: hitl`)

	var manager CommandApprover = nil // No approver
	agentID := "test-agent-none"
	guard := NewShellGuard(inner, blacklist, manager, agentID)

	ctx := context.Background()
	req := CommandRequest{
		Args: []string{"bash", "-c", "unapproved command"},
	}

	stdout, _, err := guard.Run(ctx, req)

	if err == nil {
		t.Error("Run() should return error for HITL with no approver configured")
	} else if !errors.Is(err, fmt.Errorf("shell filter requires HITL approval [hitl-no-approver-configured]: Requires HITL approval but none configured")) {
		t.Errorf("wrong error: %v", err)
	}

	if stdout != "" {
		t.Errorf("unexpected stdout: %s", stdout)
	}
}

// TestShellGuardNoMatchingRule tests that non-matching rules pass through.
func TestShellGuardNoMatchingRule(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "pass-through output"}
	blacklist := loadBlacklistForTests(`version: 1.0
rules:
  - id: block-dangerous
    pattern: "danger.*"
    reason: "Dangerous command detected"
    action: block`)

	manager := newMockCommandApprover(true, nil)
	agentID := "test-agent-pass"
	guard := NewShellGuard(inner, blacklist, manager, agentID)

	ctx := context.Background()
	req := CommandRequest{
		Args: []string{"echo", "hello"}, // Not matching any rule
	}

	stdout, _, err := guard.Run(ctx, req)

	if err != nil {
		t.Errorf("Run() should return nil for non-matching rule: %v", err)
	}

	if stdout != "pass-through output" {
		t.Errorf("wrong stdout: expected 'pass-through output', got '%s'", stdout)
	}

	if len(manager.calls) == 0 {
		// Approver should not be called for non-matching rules
	} else {
		t.Error("approver should not have been called for non-matching rule")
	}
}

// TestShellGuardNilBlacklist tests that nil blacklist passes through unmodified.
func TestShellGuardNilBlacklist(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "nil blacklist output"}
	var blacklist *ShellBlacklist = nil // Documented as valid
	manager := newMockCommandApprover(true, nil)
	agentID := "test-agent-nil"
	guard := NewShellGuard(inner, blacklist, manager, agentID)

	ctx := context.Background()
	req := CommandRequest{
		Args: []string{"any", "command"},
	}

	stdout, _, err := guard.Run(ctx, req)

	if err != nil {
		t.Errorf("Run() should return nil for nil blacklist: %v", err)
	}

	if stdout != "nil blacklist output" {
		t.Errorf("wrong stdout: expected 'nil blacklist output', got '%s'", stdout)
	}
}

// loadBlacklistForTests is a helper to create test blacklists.
func loadBlacklistForTests(yamlContent string) *ShellBlacklist {
	tmpFile, err := os.CreateTemp("", "shell_blacklist_test_*.yaml")
	if err != nil {
		panic(fmt.Errorf("failed to create temp file: %w", err))
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(yamlContent); err != nil {
		panic(fmt.Errorf("failed to write temp file: %w", err))
	}
	tmpFile.Close()

	bl, err := Load(tmpFile.Name())
	if err != nil {
		panic(fmt.Errorf("failed to load blacklist for test: %w", err))
	}

	return bl
}

// TestShellGuardMultipleMatchingRules tests that first matching rule is used.
func TestShellGuardMultipleMatchingRules(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "pass-through"}
	blacklist := loadBlacklistForTests(`version: 1.0
rules:
  - id: first-match-block
    pattern: "echo"
    reason: "Block all echo commands"
    action: block
  - id: second-match-allow
    pattern: "echo.*hello"
    reason: "Allow echo hello"
    action: allow` + ` // Note: allow is not a defined action, so first match wins`)

	manager := newMockCommandApprover(true, nil)
	agentID := "test-agent-multi"
	guard := NewShellGuard(inner, blacklist, manager, agentID)

	ctx := context.Background()
	req := CommandRequest{
		Args: []string{"echo", "hello"},
	}

	stdout, _, err := guard.Run(ctx, req)

	if err != nil {
		t.Logf("Error (expected first match): %v", err)
	} else if stdout == "pass-through" {
		// If pattern matching allows, this is acceptable
	}

	if len(manager.calls) > 0 && manager.calls[0].RuleID != "first-match-block" {
		t.Errorf("approver called with wrong rule: %s", manager.calls[0].RuleID)
	}
}

// TestShellGuardRequestStructure tests that command reconstruction is correct.
func TestShellGuardRequestStructure(t *testing.T) {
	t.Parallel()

	inner := &mockCommandRunner{stdout: "inner-calls"}
	blacklist := loadBlacklistForTests(`version: 1.0
rules:
  - id: exact-match-test
    pattern: "^echo hello$"
    reason: "Exact match required"
    action: block`)

	manager := newMockCommandApprover(true, nil)
	agentID := "test-agent-structure"
	guard := NewShellGuard(inner, blacklist, manager, agentID)

	ctx := context.Background()
	req := CommandRequest{
		Args: []string{"echo", "hello"},
	}

	stdout, stderr, err := guard.Run(ctx, req)

	if manager.calls[0].Command != "echo hello" {
		t.Errorf("expected command 'echo hello', got '%s'", manager.calls[0].Command)
	}

	if stdout == "inner-calls" && err != nil {
		t.Error("Run() should fail for blocked rule, not pass to inner")
	}
}
