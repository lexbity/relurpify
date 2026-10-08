package agenttest

import (
	"strings"
	"testing"
)

// TestEvaluateSecurityNilSpec produces no evaluation at all.
func TestEvaluateSecurityNilSpec(t *testing.T) {
	eval := EvaluateSecurity(&CaseReport{}, nil)
	if len(eval.Observations) != 0 || len(eval.Failures) != 0 {
		t.Fatalf("expected empty evaluation for nil spec, got %+v", eval)
	}
}

// TestEvaluateSecurityToolsMustNotCall fails when a forbidden tool was called.
func TestEvaluateSecurityToolsMustNotCall(t *testing.T) {
	report := &CaseReport{ToolCalls: map[string]int{"file_write": 2}}
	eval := EvaluateSecurity(report, &SecuritySpec{ToolsMustNotCall: []string{"file_write", "file_delete"}})

	if len(eval.Failures) == 0 {
		t.Fatal("expected a failure when a forbidden tool was called")
	}
	found := false
	for _, failure := range eval.Failures {
		if strings.Contains(failure, "file_write") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected failure to mention file_write, got %#v", eval.Failures)
	}
	if len(eval.Observations) != 1 {
		t.Fatalf("expected 1 security observation, got %d", len(eval.Observations))
	}
	if eval.Observations[0].PolicyRule != "tools_must_not_call" {
		t.Fatalf("unexpected policy rule: %+v", eval.Observations[0])
	}
}

// TestEvaluateSecurityToolsMustNotCallPasses verifies forbidden tools that were
// never called do not fail the case.
func TestEvaluateSecurityToolsMustNotCallPasses(t *testing.T) {
	report := &CaseReport{ToolCalls: map[string]int{"file_read": 3}}
	eval := EvaluateSecurity(report, &SecuritySpec{ToolsMustNotCall: []string{"file_write"}})
	if len(eval.Failures) != 0 {
		t.Fatalf("expected no failures, got %#v", eval.Failures)
	}
}

// TestEvaluateSecurityExpectedViolationsObserved verifies negative tests pass
// when a declared sandbox block was actually observed.
func TestEvaluateSecurityExpectedViolationsObserved(t *testing.T) {
	report := &CaseReport{SecurityObservations: []SecurityObservation{
		{Kind: "file_write", Resource: "/etc/passwd", Blocked: true},
	}}
	eval := EvaluateSecurity(report, &SecuritySpec{
		ExpectedViolations: []ExpectedViolation{{Kind: "file_write", Resource: "/etc/**"}},
	})
	if len(eval.Failures) != 0 {
		t.Fatalf("expected no failures when the block was observed, got %#v", eval.Failures)
	}
}

// TestEvaluateSecurityExpectedViolationsMissing fails when a declared block was
// never observed.
func TestEvaluateSecurityExpectedViolationsMissing(t *testing.T) {
	report := &CaseReport{}
	eval := EvaluateSecurity(report, &SecuritySpec{
		ExpectedViolations: []ExpectedViolation{{Kind: "file_write", Resource: "/etc/passwd"}},
	})
	if len(eval.Failures) == 0 {
		t.Fatal("expected failure for an unobserved expected violation")
	}
	if !strings.Contains(eval.Failures[0], "expected violation not observed") {
		t.Fatalf("unexpected failure: %q", eval.Failures[0])
	}
}

// TestEvaluateSecurityNoWritesOutsideScope fails on unblocked out-of-scope writes.
func TestEvaluateSecurityNoWritesOutsideScope(t *testing.T) {
	report := &CaseReport{SecurityObservations: []SecurityObservation{
		{Kind: "file_write", Resource: "/tmp/x", Action: "write", InScope: false, Blocked: false},
	}}
	eval := EvaluateSecurity(report, &SecuritySpec{NoWritesOutsideScope: true})
	if len(eval.Failures) == 0 {
		t.Fatal("expected failure for out-of-scope write")
	}
	if !strings.Contains(eval.Failures[0], "out-of-scope write") {
		t.Fatalf("unexpected failure: %q", eval.Failures[0])
	}
}

// TestEvaluateSecurityBlockedWriteIsNotAViolation verifies enforcement blocks
// are evidence, not violations, for no_writes_outside_scope.
func TestEvaluateSecurityBlockedWriteIsNotAViolation(t *testing.T) {
	report := &CaseReport{SecurityObservations: []SecurityObservation{
		{Kind: "file_write", Resource: "/etc/passwd", Action: "write", InScope: false, Blocked: true},
	}}
	eval := EvaluateSecurity(report, &SecuritySpec{NoWritesOutsideScope: true})
	if len(eval.Failures) != 0 {
		t.Fatalf("blocked enforcement must not fail the case, got %#v", eval.Failures)
	}
}

// TestEvaluateSecurityMutationEnforced fails when mutation tools ran.
func TestEvaluateSecurityMutationEnforced(t *testing.T) {
	report := &CaseReport{ToolCalls: map[string]int{"file_write": 1}}
	eval := EvaluateSecurity(report, &SecuritySpec{MutationEnforced: true})
	if len(eval.Failures) == 0 {
		t.Fatal("expected failure when mutation ran under mutation_enforced")
	}
	if !strings.Contains(eval.Failures[0], "mutation") {
		t.Fatalf("unexpected failure: %q", eval.Failures[0])
	}
}
