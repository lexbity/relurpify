package agenttest

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/capability/fs"
)

func TestLoadExecutorCaseReportFallsBackWhenMissing(t *testing.T) {
	desc := &PreparedRunDescriptor{ExecutionDir: filepath.Join(t.TempDir(), "execution")}
	report := loadExecutorCaseReport(desc)
	if !report.Success {
		t.Fatal("missing report should fall back to a successful case report")
	}
	if report.Name != "" {
		t.Fatalf("fallback report should not fabricate a name, got %q", report.Name)
	}
}

func TestLoadExecutorCaseReportReadsPopulatedReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	written := CaseReport{
		Name:       "smoke",
		Success:    false,
		ToolCalls:  map[string]int{"file_write": 1},
		TokenUsage: TokenUsageReport{LLMCalls: 3},
	}
	if err := WriteCaseReport(path, written); err != nil {
		t.Fatal(err)
	}
	desc := &PreparedRunDescriptor{ExecutionDir: dir}
	loaded := loadExecutorCaseReport(desc)
	if loaded.Name != "smoke" || loaded.Success != false {
		t.Fatalf("unexpected loaded report: %+v", loaded)
	}
	if loaded.ToolCalls["file_write"] != 1 || loaded.TokenUsage.LLMCalls != 3 {
		t.Fatalf("telemetry-derived fields were lost: %+v", loaded)
	}
}

func TestLoadExecutorCaseReportNilDescriptor(t *testing.T) {
	report := loadExecutorCaseReport(nil)
	if !report.Success {
		t.Fatal("nil descriptor should fall back to a successful report")
	}
}

func TestAssembleCaseReportMergesTelemetryAndVerification(t *testing.T) {
	executed := CaseReport{
		Name:        "smoke",
		Success:     false,
		ToolCalls:   map[string]int{"file_read": 2},
		FailureKind: "assertion",
	}
	verification := &PreparedRunVerificationReport{
		Success: true,
		Checks: []AssertionResult{
			{AssertionID: "prepared_run.artifact[report]", Tier: "outcome", Passed: true},
		},
	}
	desc := &PreparedRunDescriptor{
		BackendProvider:       "ollama",
		BackendEndpoint:       "http://127.0.0.1:11434",
		ModelName:             "qwen2.5",
		RecordingMode:         "off",
		DerivedWorkspaceRoot:  "/tmp/ws",
		ExecutionArtifactsDir: "/tmp/ws/exec/artifacts",
	}
	layout := newRunCaseLayout(t.TempDir(), "smoke", "qwen2.5")
	now := time.Now().UTC()
	report := assembleCaseReport(desc, CaseSpec{Name: "smoke"}, ModelSpec{Name: "qwen2.5"}, layout, executed, verification, now, now.Add(2*time.Second), "output text")

	if report.Output != "output text" {
		t.Fatalf("output not preserved: %q", report.Output)
	}
	if report.Success != true {
		t.Fatal("verification success must override executed success")
	}
	if report.ArtifactsDir != desc.ExecutionArtifactsDir {
		t.Fatalf("artifacts dir = %q, want %q", report.ArtifactsDir, desc.ExecutionArtifactsDir)
	}
	if len(report.AssertionResults) != 1 {
		t.Fatalf("expected merged assertion results, got %#v", report.AssertionResults)
	}
}

func TestApplyOSBEvaluationFailsOnSecurityViolation(t *testing.T) {
	report := CaseReport{
		Success:   true,
		ToolCalls: map[string]int{"file_write": 1},
	}
	c := CaseSpec{Expect: ExpectSpec{Security: &SecuritySpec{
		ToolsMustNotCall: []string{"file_write"},
	}}}
	result := applyOSBEvaluation(report, c, &PreparedRunDescriptor{})
	if result.Success {
		t.Fatal("security violation should fail the case")
	}
	if result.FailureKind != "security" {
		t.Fatalf("FailureKind = %q, want %q", result.FailureKind, "security")
	}
	if len(result.SecurityObservations) == 0 {
		t.Fatal("expected a security observation for the forbidden call")
	}
	found := false
	for _, ar := range result.AssertionResults {
		if ar.Tier == "security" && !ar.Passed {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a failing security assertion result, got %#v", result.AssertionResults)
	}
}

func TestApplyOSBEvaluationBenchmarkIsAdvisory(t *testing.T) {
	report := CaseReport{
		Success:   true,
		ToolCalls: map[string]int{"file_read": 1},
	}
	c := CaseSpec{Expect: ExpectSpec{Benchmark: &BenchmarkSpec{
		ToolsExpected: []string{"go_test"},
	}}}
	result := applyOSBEvaluation(report, c, &PreparedRunDescriptor{})
	if !result.Success {
		t.Fatal("presence mismatches must not fail the case")
	}
	if len(result.BenchmarkObservations) != 1 || result.BenchmarkObservations[0].Matched {
		t.Fatalf("expected an unmatched benchmark observation, got %#v", result.BenchmarkObservations)
	}
}

func TestApplyOSBEvaluationFailsOnBenchmarkThreshold(t *testing.T) {
	report := CaseReport{
		Success:   true,
		ToolCalls: map[string]int{"file_read": 6},
	}
	c := CaseSpec{Expect: ExpectSpec{Benchmark: &BenchmarkSpec{
		MaxToolCallsHint: 4,
	}}}
	result := applyOSBEvaluation(report, c, &PreparedRunDescriptor{})
	if result.Success {
		t.Fatal("benchmark threshold violation should fail the case")
	}
	if result.FailureKind != "benchmark" {
		t.Fatalf("FailureKind = %q, want %q", result.FailureKind, "benchmark")
	}
}

func TestExecuteWritesCanonicalReportAndTelemetryArtifacts(t *testing.T) {
	ws := t.TempDir()
	desc := validDescriptorWithWorkspace(t, ws)
	desc.ExecutionArtifactsDir = filepath.Join(desc.ExecutionDir, "artifacts")
	desc.ExecutionTelemetryDir = filepath.Join(desc.ExecutionDir, "telemetry")
	desc.MaxRetries = 0

	exec := (&PreparedRunExecutor{}).
		WithRunnerOverride(fakeRunner{}).
		WithAgentOverride(&fakeAgentExecutor{failCount: 0})

	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	reportPath := caseReportPath(desc)
	if reportPath == "" {
		t.Fatal("expected a canonical case report path")
	}
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("canonical report not written at %s: %v", reportPath, err)
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var report CaseReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if report.Name != desc.CaseName {
		t.Fatalf("report name = %q, want %q", report.Name, desc.CaseName)
	}

	telemetryPath := filepath.Join(desc.ExecutionTelemetryDir, "agenttest.jsonl")
	if _, err := os.Stat(telemetryPath); err != nil {
		t.Fatalf("execution telemetry not written at %s: %v", telemetryPath, err)
	}

	if err := fs.MkdirAllSecure(desc.ExecutionArtifactsDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(desc.ExecutionArtifactsDir, "framework_perf.json")); err != nil {
		t.Fatalf("framework_perf.json not written: %v", err)
	}
}

func TestCaseArtifactsDirAndReportPathResolveSensibly(t *testing.T) {
	desc := &PreparedRunDescriptor{
		ExecutionDir:          "/run/execution",
		ExecutionArtifactsDir: "/run/execution/artifacts",
	}
	if got := caseArtifactsDir(desc); got != "/run/execution/artifacts" {
		t.Fatalf("caseArtifactsDir = %q", got)
	}
	if got := caseReportPath(desc); got != "/run/execution/report.json" {
		t.Fatalf("caseReportPath = %q", got)
	}
	empty := &PreparedRunDescriptor{}
	if got := caseReportPath(empty); got != "" {
		t.Fatalf("expected empty report path for empty descriptor, got %q", got)
	}
}
