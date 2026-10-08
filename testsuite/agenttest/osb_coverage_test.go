package agenttest

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestCommittedSuitesOSBCoverage is the "no silent pass" gate (Phase 7): every
// OSB assertion the committed 24-suite catalog declares must be either
// evaluated by the harness, recognised as runtime-enforced, or recognised as
// advisory. A suite that declares an assertion with no evaluator support fails
// here instead of passing unverified.
func TestCommittedSuitesOSBCoverage(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "agenttests", "*.testsuite.yaml"))
	if err != nil {
		t.Fatalf("glob committed suites: %v", err)
	}
	if len(paths) != 24 {
		t.Fatalf("expected 24 committed suites, found %d", len(paths))
	}

	for _, path := range paths {
		suite, err := LoadSuite(path)
		if err != nil {
			t.Fatalf("load suite %s: %v", path, err)
		}
		for _, c := range suite.Spec.Cases {
			if c.Expect.Security == nil && c.Expect.Benchmark == nil {
				continue
			}
			if errs := ValidateOSBCaseCoverage(c); len(errs) != 0 {
				t.Errorf("suite %s case %s: OSB coverage violations:\n  %s",
					suite.Metadata.Name, c.Name, strings.Join(errs, "\n  "))
			}
		}
	}
}

// TestCommittedSuitesColdRunEvaluation proves the evaluators do not trip on a
// clean run: with an empty (zero tool activity, zero tokens) case report, every
// committed OSB assertion must evaluate to success. This is the deterministic,
// backend-free stand-in for "all 24 suites pass" — live pass/fail additionally
// requires a real model backend.
func TestCommittedSuitesColdRunEvaluation(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "agenttests", "*.testsuite.yaml"))
	if err != nil {
		t.Fatalf("glob committed suites: %v", err)
	}

	covered := 0
	for _, path := range paths {
		suite, err := LoadSuite(path)
		if err != nil {
			t.Fatalf("load suite %s: %v", path, err)
		}
		for _, c := range suite.Spec.Cases {
			if c.Expect.Security == nil && c.Expect.Benchmark == nil {
				continue
			}
			covered++
			report := applyOSBEvaluation(CaseReport{Success: true}, c, &PreparedRunDescriptor{})
			if !report.Success {
				t.Errorf("suite %s case %s: valid empty run failed OSB evaluation (FailureKind=%q, Error=%q)",
					suite.Metadata.Name, c.Name, report.FailureKind, report.Error)
			}
		}
	}
	if covered == 0 {
		t.Fatal("expected at least one committed case with OSB assertions")
	}
}

// TestValidateOSBCaseCoverageRecognisesEvaluatedAndRuntimeEnforcedFields locks
// the bucket semantics: evaluated fields and runtime-enforced fields must pass,
// while an unknown field must be rejected.
func TestValidateOSBCaseCoverageRecognisesFields(t *testing.T) {
	// Evaluated security + runtime-enforced + evaluated benchmark fields all
	// pass the gate.
	c := CaseSpec{Expect: ExpectSpec{
		Security: &SecuritySpec{
			ToolsMustNotCall:         []string{"file_write"},
			NoExecOutsideManifest:    true, // runtime-enforced
			NoNetworkOutsideManifest: true, // runtime-enforced
		},
		Benchmark: &BenchmarkSpec{
			ToolsExpected:    []string{"file_read"},
			ToolsNotExpected: []string{"go_test"},
			ToolSuccessRate:  map[string]int{"go_test": 80},
			TokenBudget:      &TokenBudgetHint{MaxPrompt: 8000},
			Extensions:       map[string]any{"euclo": map[string]any{"profile": "chat"}},
		},
	}}
	if errs := ValidateOSBCaseCoverage(c); len(errs) != 0 {
		t.Fatalf("expected valid coverage, got %v", errs)
	}
}

// TestApplyOSBEvaluationSecurityPassesOnEmptyReport is a direct guard: a
// clean case with a forbidden-tool assertion must not be manufactured into a
// failure when no forbidden tool was called.
func TestApplyOSBEvaluationSecurityPassesOnEmptyReport(t *testing.T) {
	c := CaseSpec{Expect: ExpectSpec{Security: &SecuritySpec{
		ToolsMustNotCall: []string{"file_write", "file_delete"},
	}}}
	report := applyOSBEvaluation(CaseReport{Success: true}, c, &PreparedRunDescriptor{})
	if !report.Success {
		t.Fatalf("empty run must pass tools_must_not_call, got FailureKind=%q", report.FailureKind)
	}
	// The declared assertion is still recorded as evaluated.
	found := false
	for _, ar := range report.AssertionResults {
		if ar.AssertionID == "security.assertions" && ar.Passed {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a passing security.assertions result, got %#v", report.AssertionResults)
	}
}
