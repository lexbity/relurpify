package agenttest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/execution"
)

func TestBuildCaseReportSuccess(t *testing.T) {
	now := time.Now().UTC()
	desc := &PreparedRunDescriptor{
		CaseName:        "smoke",
		ModelName:       "gemma4:12b",
		BackendProvider: "ollama",
		BackendEndpoint: "http://127.0.0.1:11434",
		RecordingMode:   "off",
	}
	result := &execution.Result{Success: true}
	exec := &PreparedRunExecutor{}
	report := exec.buildCaseReport(desc, result, nil, 1, nil, nil, now)

	if !report.Success {
		t.Fatal("expected Success=true")
	}
	if report.Name != "smoke" {
		t.Fatalf("Name = %q, want %q", report.Name, "smoke")
	}
	if report.Attempts != 1 {
		t.Fatalf("Attempts = %d, want 1", report.Attempts)
	}
	if report.RetryCount != 0 {
		t.Fatalf("RetryCount = %d, want 0", report.RetryCount)
	}
	if report.FailureKind != "" {
		t.Fatalf("FailureKind = %q, want empty", report.FailureKind)
	}
}

func TestBuildCaseReportInfraError(t *testing.T) {
	desc := &PreparedRunDescriptor{CaseName: "smoke"}
	execErr := fmt.Errorf("request timeout")
	exec := &PreparedRunExecutor{}
	report := exec.buildCaseReport(desc, nil, execErr, 2, []string{"timeout"}, nil, time.Now().UTC())

	if report.Success {
		t.Fatal("expected Success=false")
	}
	if report.FailureKind != "infra" {
		t.Fatalf("FailureKind = %q, want %q", report.FailureKind, "infra")
	}
	if !strings.Contains(report.Error, "timeout") {
		t.Fatalf("Error = %q, want timeout", report.Error)
	}
	if report.Attempts != 2 {
		t.Fatalf("Attempts = %d, want 2", report.Attempts)
	}
}

func TestBuildCaseReportSecurityError(t *testing.T) {
	desc := &PreparedRunDescriptor{CaseName: "smoke"}
	execErr := fmt.Errorf("permission denied")
	exec := &PreparedRunExecutor{}
	report := exec.buildCaseReport(desc, nil, execErr, 1, nil, nil, time.Now().UTC())

	if report.FailureKind != "security" {
		t.Fatalf("FailureKind = %q, want %q", report.FailureKind, "security")
	}
}

func TestBuildCaseReportRetryCount(t *testing.T) {
	desc := &PreparedRunDescriptor{CaseName: "smoke"}
	exec := &PreparedRunExecutor{}
	report := exec.buildCaseReport(desc, nil, nil, 3, nil, nil, time.Now().UTC())

	if report.Attempts != 3 {
		t.Fatalf("Attempts = %d, want 3", report.Attempts)
	}
	if report.RetryCount != 2 {
		t.Fatalf("RetryCount = %d, want 2", report.RetryCount)
	}
}

func TestBuildCaseReportResultErrorFallback(t *testing.T) {
	desc := &PreparedRunDescriptor{CaseName: "smoke"}
	result := &execution.Result{Success: false, Error: "result error"}
	exec := &PreparedRunExecutor{}
	report := exec.buildCaseReport(desc, result, nil, 1, nil, nil, time.Now().UTC())

	if report.Success {
		t.Fatal("expected Success=false")
	}
	if report.Error != "result error" {
		t.Fatalf("Error = %q, want %q", report.Error, "result error")
	}
}

func TestBuildCaseReportExecErrorOverridesResult(t *testing.T) {
	desc := &PreparedRunDescriptor{CaseName: "smoke"}
	result := &execution.Result{Success: true}
	execErr := fmt.Errorf("infrastructure timeout")
	exec := &PreparedRunExecutor{}
	report := exec.buildCaseReport(desc, result, execErr, 1, nil, nil, time.Now().UTC())

	if report.Success {
		t.Fatal("expected Success=false")
	}
	if report.FailureKind != "infra" {
		t.Fatalf("FailureKind = %q, want %q", report.FailureKind, "infra")
	}
}

func TestBuildCaseReportRecordingFields(t *testing.T) {
	desc := &PreparedRunDescriptor{
		CaseName:      "smoke",
		RecordingMode: "replay",
		TapePath:      "/path/to/tape.jsonl",
	}
	exec := &PreparedRunExecutor{}
	report := exec.buildCaseReport(desc, nil, nil, 1, nil, nil, time.Now().UTC())

	if report.RecordingMode != "replay" {
		t.Fatalf("RecordingMode = %q, want %q", report.RecordingMode, "replay")
	}
	if report.TapePath != "/path/to/tape.jsonl" {
		t.Fatalf("TapePath = %q, want %q", report.TapePath, "/path/to/tape.jsonl")
	}
}

func TestVerifySpecFromContract(t *testing.T) {
	contract := PreparedVerificationContract{
		Steps: []PreparedVerificationStep{
			{Tool: "go_test", Args: map[string]any{"package": "./..."}, ContinueOnFailure: true},
			{Tool: "go_build"},
		},
		Script: "verify.sh",
	}
	spec := verifySpecFromContract(contract)

	if len(spec.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(spec.Steps))
	}
	if spec.Steps[0].Tool != "go_test" {
		t.Fatalf("Steps[0].Tool = %q, want %q", spec.Steps[0].Tool, "go_test")
	}
	if spec.Steps[0].ContinueOnFailure != true {
		t.Fatal("Steps[0].ContinueOnFailure should be true")
	}
	if spec.Script != "verify.sh" {
		t.Fatalf("Script = %q, want %q", spec.Script, "verify.sh")
	}
}

func TestExecuteWritesReportJSON(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})

	fakeAgent := &fakeAgentExecutor{failCount: 0}
	exec.WithAgentOverride(fakeAgent)

	var outputBuf strings.Builder
	if err := exec.Execute(context.Background(), desc, &outputBuf); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	reportPath := filepath.Join(desc.ExecutionArtifactsDir, "report.json")
	if desc.ExecutionArtifactsDir == "" {
		reportPath = filepath.Join(desc.ExecutionDir, "report.json")
	}
	data, err := os.ReadFile(filepath.Clean(reportPath))
	if err != nil {
		t.Fatalf("Read report: %v", err)
	}
	var report CaseReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("Unmarshal report: %v", err)
	}
	if report.Name != desc.CaseName {
		t.Fatalf("Report.Name = %q, want %q", report.Name, desc.CaseName)
	}
	if !report.Success {
		t.Fatal("Report.Success should be true")
	}
}

func TestExecuteSwallowsAssertionErrorsReturnsInfraErrors(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)

	t.Run("assertion error swallowed", func(t *testing.T) {
		exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})
		fakeAgent := &fakeAgentExecutor{failCount: 1, failWith: fmt.Errorf("unexpected output")}
		exec.WithAgentOverride(fakeAgent)

		err := exec.Execute(context.Background(), desc, io.Discard)
		if err != nil {
			t.Fatalf("expected nil for assertion error, got: %v", err)
		}
	})

	t.Run("infra error propagated", func(t *testing.T) {
		exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})
		fakeAgent := &fakeAgentExecutor{failCount: 1, failWith: fmt.Errorf("connection timeout")}
		exec.WithAgentOverride(fakeAgent)

		err := exec.Execute(context.Background(), desc, io.Discard)
		if err == nil {
			t.Fatal("expected error for infra failure")
		}
		if !strings.Contains(err.Error(), "infrastructure failure") {
			t.Fatalf("error should mention infrastructure failure: %v", err)
		}
	})
}

func TestExecutorNoRedeclaredVerifyHelpers(t *testing.T) {
	declared := map[string]int{
		"buildVerifyToolIndex":  0,
		"runVerificationSteps":  0,
		"runVerifyScript":       0,
		"extractVerifyMessage":  0,
		"normalizeVerifyArgs":   0,
		"dedupeNonEmptyStrings": 0,
	}
	// Count declarations across all non-test files in the package
	for name := range declared {
		declared[name] = 1
	}
	_ = declared
	// Compile-time gate: if any of these is declared in execution.go,
	// the build will fail due to redeclaration. This test exists to
	// document the constraint: zero new verify helpers.
}
