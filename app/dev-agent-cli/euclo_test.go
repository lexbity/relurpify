package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRootCmdIncludesEucloGroup(t *testing.T) {
	cmd := NewRootCmd()
	if found, _, err := cmd.Find([]string{"euclo"}); err != nil {
		t.Fatal(err)
	} else if found == nil {
		t.Fatal("expected euclo command group to be registered")
	}
}

func TestEucloCapabilitiesListJSON(t *testing.T) {
	withCLIState(t, t.TempDir())
	cmd := newEucloCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"capabilities", "list", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var entries []CapabilityCatalogEntry
	if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
		t.Fatalf("output was not valid JSON: %v\n%s", err, out.String())
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one capability entry")
	}
	if entries[0].ID == "" {
		t.Fatal("expected capability IDs to be populated")
	}
}

func TestEucloTriggersResolveAndFire(t *testing.T) {
	withCLIState(t, t.TempDir())

	resolveCmd := newEucloCmd()
	var resolveOut bytes.Buffer
	resolveCmd.SetOut(&resolveOut)
	resolveCmd.SetErr(&resolveOut)
	resolveCmd.SetArgs([]string{"triggers", "resolve", "--mode", "chat", "--text", "implement this", "--json"})
	if err := resolveCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var resolution EucloTriggerResolution
	if err := json.Unmarshal(resolveOut.Bytes(), &resolution); err != nil {
		t.Fatalf("resolve output was not valid JSON: %v\n%s", err, resolveOut.String())
	}
	if !resolution.Matched || resolution.Trigger == nil {
		t.Fatalf("expected a matched trigger, got %+v", resolution)
	}

	fireCmd := newEucloCmd()
	var fireOut bytes.Buffer
	fireCmd.SetOut(&fireOut)
	fireCmd.SetErr(&fireOut)
	fireCmd.SetArgs([]string{"triggers", "fire", "--mode", "planning", "--phrase", "alternatives", "--json"})
	if err := fireCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var fired EucloTriggerFireResult
	if err := json.Unmarshal(fireOut.Bytes(), &fired); err != nil {
		t.Fatalf("fire output was not valid JSON: %v\n%s", err, fireOut.String())
	}
	if !fired.Matched || len(fired.Artifacts) == 0 {
		t.Fatalf("expected trigger fire to produce artifacts, got %+v", fired)
	}
}

func TestEucloJourneyRunExecutesLocalScript(t *testing.T) {
	ws := t.TempDir()
	withCLIState(t, ws)

	scriptPath := filepath.Join(ws, "journey.yaml")
	script := EucloJourneyScript{
		ScriptVersion: "v1alpha1",
		InitialMode:   "planning",
		InitialContext: map[string]any{
			"current_mode": "planning",
		},
		Steps: []EucloJourneyStep{
			{Kind: "trigger.fire", Text: "alternatives"},
			{Kind: "context.add", Key: "note", Value: "hello"},
			{Kind: "artifact.expect", Expected: "euclo:design.alternatives"},
		},
		ExpectedTerminalState: map[string]any{
			"note":         "hello",
			"current_mode": "planning",
		},
	}
	data, err := yaml.Marshal(script)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newEucloCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"journey", "run", "--file", scriptPath, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var report EucloJourneyReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("journey output was not valid JSON: %v\n%s", err, out.String())
	}
	if !report.Success {
		t.Fatalf("expected journey to succeed, got failures: %v", report.Failures)
	}
	if report.FinalMode != "planning" {
		t.Fatalf("final mode = %q, want planning", report.FinalMode)
	}
	if got := report.TerminalState["note"]; got != "hello" {
		t.Fatalf("terminal state note = %v, want hello", got)
	}
}

func TestEucloJourneyRunLoadsCodeTransitionFixture(t *testing.T) {
	withCLIState(t, t.TempDir())
	script, err := loadEucloJourneyScript(filepath.Join("testdata", "euclo", "journey_code_transition.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := newEucloCommandRunner().RunJourney(context.Background(), script)
	if err != nil {
		t.Fatal(err)
	}
	if !run.Success {
		t.Fatalf("expected code transition fixture to succeed, got %v", run.Failures)
	}
	if run.FinalMode != "debug" {
		t.Fatalf("final mode = %q, want debug", run.FinalMode)
	}
}

func TestEucloJourneyReplayFixtureCapturesTranscriptAndTransitions(t *testing.T) {
	withCLIState(t, t.TempDir())
	script, err := loadEucloJourneyScript(filepath.Join("testdata", "euclo", "journey_planning_replay.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := newEucloCommandRunner().RunJourney(context.Background(), script)
	if err != nil {
		t.Fatal(err)
	}
	if !run.Success {
		t.Fatalf("expected replay fixture to succeed, got %v", run.Failures)
	}
	if run.RunMode != "replay" {
		t.Fatalf("run mode = %q, want replay", run.RunMode)
	}
	if len(run.Transcript) != len(run.Steps) {
		t.Fatalf("transcript entries = %d, want %d", len(run.Transcript), len(run.Steps))
	}
	if len(run.Frames) != len(run.Steps) {
		t.Fatalf("frames = %d, want %d", len(run.Frames), len(run.Steps))
	}
	if len(run.Responses) != len(run.Steps) {
		t.Fatalf("responses = %d, want %d", len(run.Responses), len(run.Steps))
	}
	if len(run.Transitions) != 1 {
		t.Fatalf("transitions = %d, want 1", len(run.Transitions))
	}
	if got := run.Transcript[0].Kind; got != "trigger.fire" {
		t.Fatalf("first transcript kind = %q, want trigger.fire", got)
	}
	if got := run.Transcript[0].Message; got != "step applied" {
		t.Fatalf("first transcript message = %q, want step applied", got)
	}
	if got := run.Frames[0].Kind; got != "proposal" {
		t.Fatalf("first frame kind = %q, want proposal", got)
	}
	if got := run.Responses[0].ActionID; got != "continue" {
		t.Fatalf("first response action = %q, want continue", got)
	}
}

func TestLoadEucloJourneyScriptValidatesSchema(t *testing.T) {
	script, err := loadEucloJourneyScript(filepath.Join("testdata", "euclo", "journey_planning_alternatives.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if script.InitialMode != "planning" {
		t.Fatalf("initial_mode = %q, want planning", script.InitialMode)
	}
}

func TestLoadEucloJourneyScriptRejectsUnknownStepKind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte(`
script_version: v1alpha1
initial_mode: chat
steps:
  - kind: not.a.real.step
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadEucloJourneyScript(path); err == nil || !strings.Contains(err.Error(), "not.a.real.step") {
		t.Fatalf("expected unsupported step kind error, got %v", err)
	}
}

func TestEucloBenchmarkRunProducesAggregateReport(t *testing.T) {
	withCLIState(t, t.TempDir())
	matrixPath := filepath.Join("testdata", "euclo", "benchmark_matrix.yaml")
	matrix, err := loadEucloBenchmarkMatrix(matrixPath)
	if err != nil {
		t.Fatal(err)
	}
	report, err := newEucloCommandRunner().RunBenchmark(nil, matrix)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.TotalCases != 1 {
		t.Fatalf("total_cases = %d, want 1", report.Summary.TotalCases)
	}
	if report.Summary.PassedCases != 1 || report.Summary.FailedCases != 0 {
		t.Fatalf("unexpected summary: %+v", report.Summary)
	}
	if len(report.Cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(report.Cases))
	}
	if report.Cases[0].Journey == nil || !report.Cases[0].Journey.Success {
		t.Fatalf("expected journey report on benchmark row, got %+v", report.Cases[0])
	}
}

func TestEucloBenchmarkRunJSONIncludesSummary(t *testing.T) {
	withCLIState(t, t.TempDir())
	cmd := newEucloCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"benchmark", "run", "--matrix", filepath.Join("testdata", "euclo", "benchmark_matrix.yaml"), "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var report EucloBenchmarkReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("benchmark output was not valid JSON: %v\n%s", err, out.String())
	}
	if report.Summary.TotalCases != 1 {
		t.Fatalf("summary = %+v, want 1 case", report.Summary)
	}
	if !report.Success {
		t.Fatalf("expected benchmark success, got %+v", report)
	}
}

func TestLoadEucloBenchmarkMatrixSupportsExplicitSets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "matrix.yaml")
	if err := os.WriteFile(path, []byte(`
matrix_version: v1alpha1
name: explicit-matrix
axis_order: model-first
capabilities: ["euclo:chat.ask"]
model_set:
  - name: qwen2.5-coder:14b
    endpoint: http://localhost:11434
provider_set:
  - name: ollama
    endpoint: http://localhost:11434
    reset_strategy: model
`), 0o644); err != nil {
		t.Fatal(err)
	}
	matrix, err := loadEucloBenchmarkMatrix(path)
	if err != nil {
		t.Fatal(err)
	}
	if matrix.AxisOrder != "model-first" {
		t.Fatalf("axis_order = %q, want model-first", matrix.AxisOrder)
	}
	if len(matrix.ModelSet) != 1 || len(matrix.ProviderSet) != 1 {
		t.Fatalf("unexpected matrix sets: %+v", matrix)
	}
}

func TestEucloBaselineListJSON(t *testing.T) {
	withCLIState(t, t.TempDir())
	cmd := newEucloCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"baseline", "list", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var entries []CapabilityCatalogEntry
	if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
		t.Fatalf("baseline output was not valid JSON: %v\n%s", err, out.String())
	}
	if len(entries) == 0 {
		t.Fatal("expected baseline-eligible capability entries")
	}
	if !entries[0].BaselineEligible {
		t.Fatalf("expected baseline list entry to be baseline eligible: %+v", entries[0])
	}
}

func TestEucloBaselineRunJSON(t *testing.T) {
	withCLIState(t, t.TempDir())
	cmd := newEucloCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"baseline", "run", "--capability", "euclo:chat.ask", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var report EucloBaselineReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("baseline run output was not valid JSON: %v\n%s", err, out.String())
	}
	if !report.Success {
		t.Fatalf("expected baseline success, got %+v", report)
	}
	if report.Exact != true || !report.BenchmarkAggregationDisabled {
		t.Fatalf("unexpected baseline flags: %+v", report)
	}
	if len(report.Capabilities) != 1 {
		t.Fatalf("capabilities = %d, want 1", len(report.Capabilities))
	}
	if report.Capabilities[0].Capability == nil || report.Capabilities[0].Capability.ID != "euclo:chat.ask" {
		t.Fatalf("unexpected capability snapshot: %+v", report.Capabilities[0])
	}
}
