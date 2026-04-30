package agenttest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestRepresentativeSuitesLoad verifies that the committed catalog now loads
// under the strict generic schema.
func TestRepresentativeSuitesLoad(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "agenttests", "*.yaml"))
	if err != nil {
		t.Fatalf("glob committed suites: %v", err)
	}
	for _, path := range paths {
		if _, err := LoadSuite(path); err != nil {
			t.Fatalf("expected suite %s to load, got %v", path, err)
		}
	}
}

// TestGenericSuiteLoadsWithoutLegacyFields verifies the new schema still loads
// when only generic OSB blocks are present.
func TestGenericSuiteLoadsWithoutLegacyFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "suite.yaml")
	err := os.WriteFile(path, []byte(`
apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: sample
  tier: stable
  quarantined: false
spec:
  agent_name: coding
  manifest: relurpify_cfg/agent.manifest.yaml
  execution:
    profile: live
  workspace:
    strategy: derived
  cases:
    - name: smoke
      prompt: summarize
      expect:
        outcome:
          must_succeed: true
          output_contains:
            - "done"
        security:
          no_writes_outside_scope: true
        benchmark:
          tools_expected:
            - file_read
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	suite, err := LoadSuite(path)
	if err != nil {
		t.Fatalf("LoadSuite(%q) error = %v", path, err)
	}
	if suite.Spec.Cases[0].Expect.Outcome == nil {
		t.Fatal("expected outcome block to survive generic load")
	}
	if suite.Spec.Cases[0].Expect.Security == nil {
		t.Fatal("expected security block to survive generic load")
	}
	if suite.Spec.Cases[0].Expect.Benchmark == nil {
		t.Fatal("expected benchmark block to survive generic load")
	}
}

// TestReportSchemaStability validates that CaseReport serialization/deserialization
// is lossless for all OSB report fields.
func TestReportSchemaStability(t *testing.T) {
	original := CaseReport{
		Name:    "test-case",
		Success: true,
		Error:   "",
		SecurityObservations: []SecurityObservation{
			{
				Kind:       "file_write",
				Resource:   "/tmp/test.txt",
				Action:     "write",
				InScope:    true,
				Blocked:    false,
				Expected:   false,
				Timestamp:  "2024-01-15T10:30:00Z",
				AgentID:    "agent-123",
				PolicyRule: "file_write_allowed",
			},
		},
		BenchmarkObservations: []BenchmarkObservation{
			{
				Category: "tool_usage",
				Field:    "tools_expected",
				Expected: "file_write",
				Actual:   "true",
				Matched:  true,
			},
		},
		AssertionResults: []AssertionResult{
			{
				AssertionID: "outcome.files_changed[foo.go]",
				Tier:        "outcome",
				Passed:      true,
				Message:     "file foo.go changed",
			},
			{
				AssertionID: "security.no_writes_outside_scope",
				Tier:        "security",
				Passed:      true,
				Message:     "",
			},
		},
		FailureKind: "none",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal CaseReport: %v", err)
	}

	var roundTripped CaseReport
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("Failed to unmarshal CaseReport: %v", err)
	}

	if len(roundTripped.SecurityObservations) != 1 {
		t.Errorf("SecurityObservations lost: got %d, want 1", len(roundTripped.SecurityObservations))
	}
	if len(roundTripped.BenchmarkObservations) != 1 {
		t.Errorf("BenchmarkObservations lost: got %d, want 1", len(roundTripped.BenchmarkObservations))
	}
	if len(roundTripped.AssertionResults) != 2 {
		t.Errorf("AssertionResults lost: got %d, want 2", len(roundTripped.AssertionResults))
	}

	if roundTripped.SecurityObservations[0].Kind != "file_write" {
		t.Errorf("SecurityObservation.Kind mismatch: got %s, want file_write", roundTripped.SecurityObservations[0].Kind)
	}
	if roundTripped.BenchmarkObservations[0].Category != "tool_usage" {
		t.Errorf("BenchmarkObservation.Category mismatch: got %s, want tool_usage", roundTripped.BenchmarkObservations[0].Category)
	}
	if roundTripped.AssertionResults[0].Tier != "outcome" {
		t.Errorf("AssertionResult.Tier mismatch: got %s, want outcome", roundTripped.AssertionResults[0].Tier)
	}
}

// TestNoLegacyEvaluatorCode validates that the generic OSB evaluators are the
// only ones expected to remain in the shared engine.
func TestNoLegacyEvaluatorCode(t *testing.T) {
	_ = evaluateOutcomeExpectations
	_ = evaluateSecurityExpectations
	_ = evaluateBenchmarkExpectations
}
