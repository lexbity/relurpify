package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

func TestAgentTestRunDefaultsSkipASTIndex(t *testing.T) {
	cmd := newAgentTestRunCmd()
	flag := cmd.Flags().Lookup("skip-ast-index")
	if flag == nil {
		t.Fatal("expected skip-ast-index flag")
	}
	if flag.DefValue != "true" {
		t.Fatalf("expected skip-ast-index default true, got %q", flag.DefValue)
	}
}

func TestAgentTestRefreshDefaultsSkipASTIndex(t *testing.T) {
	cmd := newAgentTestRefreshCmd()
	flag := cmd.Flags().Lookup("skip-ast-index")
	if flag == nil {
		t.Fatal("expected skip-ast-index flag")
	}
	if flag.DefValue != "true" {
		t.Fatalf("expected skip-ast-index default true, got %q", flag.DefValue)
	}
}

func TestFilterAgentTestSuiteCasesByTag(t *testing.T) {
	suite := &agenttest.Suite{
		Spec: agenttest.SuiteSpec{
			Cases: []agenttest.CaseSpec{
				{Name: "recovery_case", Tags: []string{"recovery"}},
				{Name: "stable_case", Tags: []string{"stable"}},
			},
		},
	}

	filtered, err := filterAgentTestSuiteCases(suite, "", []string{"recovery"})
	if err != nil {
		t.Fatalf("filterAgentTestSuiteCases: %v", err)
	}
	if len(filtered.Spec.Cases) != 1 || filtered.Spec.Cases[0].Name != "recovery_case" {
		t.Fatalf("unexpected filtered cases: %+v", filtered.Spec.Cases)
	}
}

func TestFilterAgentTestSuiteCasesSelectsNamedCase(t *testing.T) {
	suite := &agenttest.Suite{
		Spec: agenttest.SuiteSpec{
			Cases: []agenttest.CaseSpec{
				{Name: "recovery_case", Tags: []string{"recovery"}},
				{Name: "stable_case", Tags: []string{"stable"}},
			},
		},
	}

	filtered, err := filterAgentTestSuiteCases(suite, "stable_case", nil)
	if err != nil {
		t.Fatalf("filterAgentTestSuiteCases: %v", err)
	}
	if len(filtered.Spec.Cases) != 1 || filtered.Spec.Cases[0].Name != "stable_case" {
		t.Fatalf("unexpected filtered cases: %+v", filtered.Spec.Cases)
	}
}

func TestFilterAgentTestSuiteCasesRejectsCaseMissAfterTagFilter(t *testing.T) {
	suite := &agenttest.Suite{
		Spec: agenttest.SuiteSpec{
			Cases: []agenttest.CaseSpec{
				{Name: "recovery_case", Tags: []string{"recovery"}},
				{Name: "stable_case", Tags: []string{"stable"}},
			},
		},
	}

	_, err := filterAgentTestSuiteCases(suite, "stable_case", []string{"recovery"})
	if err == nil || !strings.Contains(err.Error(), `case "stable_case" not found after applying tags recovery`) {
		t.Fatalf("expected tag-filtered case miss, got %v", err)
	}
}

func TestPromoteAgentTestRunCopiesPassingTapeToGoldenDir(t *testing.T) {
	workspace := t.TempDir()
	suitePath := filepath.Join(workspace, "testsuite", "agenttests", "euclo.code.testsuite.yaml")
	if err := os.MkdirAll(filepath.Dir(suitePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(suitePath, []byte(`apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: euclo.code
spec:
  agent_name: euclo
  manifest: relurpify_cfg/agent.manifest.yaml
  cases:
    - name: basic_edit_task
      prompt: hello
`), 0o644); err != nil {
		t.Fatal(err)
	}

	runDir := filepath.Join(workspace, "relurpify_cfg", "test_runs", "euclo", "run-1")
	artifactsDir := filepath.Join(runDir, "artifacts", "basic_edit_task__qwen2_5_coder_14b")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactsDir, "tape.jsonl"), []byte(`{"kind":"_header","request":{"header":{"kind":"_header","model_name":"qwen2.5-coder:14b"}}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactsDir, "interaction.tape.jsonl"), []byte(`{"kind":"proposal","phase":"scope"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := agenttest.SuiteReport{
		Cases: []agenttest.CaseReport{{
			Name:         "basic_edit_task",
			Model:        "qwen2.5-coder:14b",
			Success:      true,
			ArtifactsDir: artifactsDir,
		}},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if err := promoteAgentTestRun(workspace, suitePath, runDir, "basic_edit_task", false, &out); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(workspace, "testsuite", "agenttests", "tapes", "euclo.code", "basic_edit_task__qwen2_5_coder_14b.tape.jsonl")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("expected promoted tape at %s: %v", dest, err)
	}
	destInteraction := filepath.Join(workspace, "testsuite", "agenttests", "tapes", "euclo.code", "basic_edit_task__qwen2_5_coder_14b.interaction.tape.jsonl")
	if _, err := os.Stat(destInteraction); err != nil {
		t.Fatalf("expected promoted interaction tape at %s: %v", destInteraction, err)
	}
	destBaseline := filepath.Join(workspace, "testsuite", "agenttests", "tapes", "euclo.code", "basic_edit_task__qwen2_5_coder_14b.baseline.json")
	if _, err := os.Stat(destBaseline); err != nil {
		t.Fatalf("expected promoted baseline at %s: %v", destBaseline, err)
	}
	if !strings.Contains(out.String(), "promoted") {
		t.Fatalf("expected promote output, got %q", out.String())
	}
}

func TestReadTapeHeaderReturnsNilForHeaderlessTape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "headerless.tape.jsonl")
	if err := os.WriteFile(path, []byte(`{"kind":"generate"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	header, err := readTapeHeader(path)
	if err != nil {
		t.Fatal(err)
	}
	if header != nil {
		t.Fatalf("expected nil header, got %+v", header)
	}
}

func TestFormatTapeStatusReportsMissingHeader(t *testing.T) {
	status := agentTestSurface.FormatTapeStatus(&llm.TapeInspection{}, "qwen2.5-coder:14b", time.Now())
	if status != "missing header" {
		t.Fatalf("expected missing header status, got %q", status)
	}
}

func TestReportAgentTestTapesShowsAgeAndMissingCoverage(t *testing.T) {
	workspace := t.TempDir()
	suitePath := filepath.Join(workspace, "testsuite", "agenttests", "euclo.code.testsuite.yaml")
	if err := os.MkdirAll(filepath.Dir(suitePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(suitePath, []byte(`apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: euclo.code
spec:
  agent_name: euclo
  manifest: relurpify_cfg/agent.manifest.yaml
  models:
    - name: qwen2.5-coder:14b
  cases:
    - name: basic_edit_task
      prompt: hello
    - name: code_no_mode_hint
      prompt: world
`), 0o644); err != nil {
		t.Fatal(err)
	}

	tapeDir := filepath.Join(workspace, "testsuite", "agenttests", "tapes", "euclo.code")
	if err := os.MkdirAll(tapeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tapeDir, "basic_edit_task__qwen2_5_coder_14b.tape.jsonl"), []byte(
		`{"kind":"_header","request":{"header":{"kind":"_header","model_name":"qwen2.5-coder:14b","suite_name":"euclo.code","case_name":"basic_edit_task","recorded_at":"2026-02-01T00:00:00Z"}}}`+"\n"+
			`{"kind":"generate","timestamp":"2026-02-01T00:00:01Z","fingerprint":"abc123","request":{"prompt":"hello"},"response":{"text":"ok","finish_reason":"stop"}}`+"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	now := time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)
	if err := reportAgentTestTapes(workspace, []string{suitePath}, &out, now); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "Suite: euclo.code") {
		t.Fatalf("expected suite header, got %q", got)
	}
	if !strings.Contains(got, "basic_edit_task:") || !strings.Contains(got, "qwen2.5-coder:14b  recorded 2026-02-01  ! 45 days old") {
		t.Fatalf("expected recorded tape age line, got %q", got)
	}
	if !strings.Contains(got, "code_no_mode_hint:\n    (no golden tape)") {
		t.Fatalf("expected missing tape line, got %q", got)
	}
}

func TestPromoteAgentTestRunWritesLineage(t *testing.T) {
	workspace := t.TempDir()
	suitePath := filepath.Join(workspace, "testsuite", "agenttests", "euclo.code.testsuite.yaml")
	if err := os.MkdirAll(filepath.Dir(suitePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(suitePath, []byte(`apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: euclo.code
  classification: capability
spec:
  agent_name: euclo
  manifest: relurpify_cfg/agent.manifest.yaml
  cases:
    - name: basic_edit_task
      prompt: hello
`), 0o644); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(workspace, "relurpify_cfg", "test_runs", "euclo", "run-1")
	artifactsDir := filepath.Join(runDir, "artifacts", "basic_edit_task__qwen2_5_coder_14b")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactsDir, "tape.jsonl"), []byte(`{"kind":"_header","request":{"header":{"kind":"_header","model_name":"qwen2.5-coder:14b"}}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactsDir, "interaction.tape.jsonl"), []byte(`{"kind":"proposal","phase":"scope"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := agenttest.SuiteReport{
		Cases: []agenttest.CaseReport{{
			Name:         "basic_edit_task",
			Model:        "qwen2.5-coder:14b",
			Provider:     "ollama",
			Success:      true,
			ArtifactsDir: artifactsDir,
		}},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if err := promoteAgentTestRun(workspace, suitePath, runDir, "basic_edit_task", false, &out); err != nil {
		t.Fatal(err)
	}
	lineage := filepath.Join(workspace, "testsuite", "agenttests", "tapes", "euclo.code", "basic_edit_task__qwen2_5_coder_14b.promotion.json")
	if _, err := os.Stat(lineage); err != nil {
		t.Fatalf("expected promotion lineage at %s: %v", lineage, err)
	}
}
