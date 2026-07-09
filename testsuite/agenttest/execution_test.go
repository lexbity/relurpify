package agenttest

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func TestPreparedRunDescriptorCarriesRecordingPlan(t *testing.T) {
	workspace := t.TempDir()
	suitePath := filepath.Join(workspace, "suite.yaml")
	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agent.yaml")
	if err := fs.MkdirAllSecure(filepath.Dir(manifestPath)); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFileSecure(manifestPath, []byte(`schema: relurpify/agent/v1
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: euclo
spec:
  image: ghcr.io/lexcodex/relurpify/runtime:latest
  runtime: gvisor
  permissions:
    filesystem:
      - action: fs:read
        path: ${workspace}/**
        justification: read workspace
  agent:
    implementation: euclo
    mode: primary
    model:
      provider: ollama
      name: gemma4:12b
`)); err != nil {
		t.Fatal(err)
	}

	suite := &Suite{
		SourcePath: suitePath,
		Metadata:   SuiteMeta{Name: "euclo.code"},
		Spec: SuiteSpec{
			AgentName: "euclo",
			Manifest:  filepath.ToSlash(filepath.Join(config.DirName, "agent.yaml")),
			Recording: RecordingSpec{
				Mode: "replay",
			},
			Models: []ModelSpec{{
				Name:     "gemma4:12b",
				Provider: "ollama",
				Endpoint: "http://127.0.0.1:11434",
			}},
			Execution: SuiteExecutionSpec{Profile: "live", Strict: true},
		},
	}
	caseSpec := CaseSpec{
		Name:   "smoke",
		Prompt: "do something",
	}
	runRoot := filepath.Join(workspace, "relurpify_cfg", "test_run", "run-replay")
	tapePath := filepath.Join(runRoot, "execution", "artifacts", "tape.jsonl")
	if err := fs.MkdirAllSecure(filepath.Dir(tapePath)); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFileSecure(tapePath, []byte(`{"kind":"_header","request":{"header":{"model_name":"gemma4:12b"}}}`)); err != nil {
		t.Fatal(err)
	}

	desc, err := BuildPreparedRunDescriptor(suite, caseSpec, suite.Spec.Models[0], RunOptions{}, workspace, runRoot, "run-replay")
	if err != nil {
		t.Fatalf("BuildPreparedRunDescriptor: %v", err)
	}
	if desc.RecordingMode != "replay" {
		t.Fatalf("RecordingMode = %q, want %q", desc.RecordingMode, "replay")
	}
	if desc.TapePath == "" {
		t.Fatal("TapePath is empty, expected non-empty for replay mode")
	}
}

func TestPreparedRunDescriptorRecordingOffWhenAbsent(t *testing.T) {
	workspace := t.TempDir()
	suitePath := filepath.Join(workspace, "suite.yaml")
	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agent.yaml")
	if err := fs.MkdirAllSecure(filepath.Dir(manifestPath)); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFileSecure(manifestPath, []byte(`schema: relurpify/agent/v1
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: euclo
spec:
  image: ghcr.io/lexcodex/relurpify/runtime:latest
  runtime: gvisor
  agent:
    implementation: euclo
    mode: primary
    model:
      provider: ollama
      name: gemma4:12b
`)); err != nil {
		t.Fatal(err)
	}

	suite := &Suite{
		SourcePath: suitePath,
		Metadata:   SuiteMeta{Name: "euclo.code"},
		Spec: SuiteSpec{
			AgentName: "euclo",
			Manifest:  filepath.ToSlash(filepath.Join(config.DirName, "agent.yaml")),
			Models: []ModelSpec{{
				Name:     "gemma4:12b",
				Provider: "ollama",
				Endpoint: "http://127.0.0.1:11434",
			}},
		},
	}
	caseSpec := CaseSpec{
		Name:   "smoke",
		Prompt: "do something",
	}
	runRoot := filepath.Join(workspace, "relurpify_cfg", "test_run", "run-off")
	desc, err := BuildPreparedRunDescriptor(suite, caseSpec, suite.Spec.Models[0], RunOptions{}, workspace, runRoot, "run-off")
	if err != nil {
		t.Fatalf("BuildPreparedRunDescriptor: %v", err)
	}
	if desc.RecordingMode != "off" {
		t.Fatalf("RecordingMode = %q, want %q", desc.RecordingMode, "off")
	}
	if desc.TapePath != "" {
		t.Fatalf("TapePath = %q, want empty", desc.TapePath)
	}
}

func TestValidateRecordingPlanReplayRequiresTape(t *testing.T) {
	desc := &PreparedRunDescriptor{
		RunID:                "test",
		SuitePath:            "/suite.yaml",
		SuiteName:            "suite",
		CaseName:             "case",
		AgentName:            "agent",
		Instruction:          "do it",
		WorkspaceRoot:        "/tmp/ws",
		RunRoot:              "/tmp/run",
		ConfigPath:           "/tmp/cfg.yaml",
		SetupDir:             "/tmp/setup",
		ExecutionDir:         "/tmp/exec",
		BackendSelection:     PreparedRunSelectionSingle,
		BackendProvider:      "ollama",
		BackendFamily:        "ollama",
		BackendEndpoint:      "http://127.0.0.1:11434",
		DerivedWorkspaceRoot: "/tmp/ws/derived",
		RecordingMode:        "replay",
		TapePath:             "",
	}
	err := validateRecordingPlan(desc)
	if err == nil {
		t.Fatal("expected error for replay mode without tape_path")
	}
	if !strings.Contains(err.Error(), "replay mode requires tape_path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRecordingPlanReplayMissingFile(t *testing.T) {
	desc := &PreparedRunDescriptor{
		RunID:                "test",
		SuitePath:            "/suite.yaml",
		SuiteName:            "suite",
		CaseName:             "case",
		AgentName:            "agent",
		Instruction:          "do it",
		WorkspaceRoot:        "/tmp/ws",
		RunRoot:              "/tmp/run",
		ConfigPath:           "/tmp/cfg.yaml",
		SetupDir:             "/tmp/setup",
		ExecutionDir:         "/tmp/exec",
		BackendSelection:     PreparedRunSelectionSingle,
		BackendProvider:      "ollama",
		BackendFamily:        "ollama",
		BackendEndpoint:      "http://127.0.0.1:11434",
		DerivedWorkspaceRoot: "/tmp/ws/derived",
		RecordingMode:        "replay",
		TapePath:             "/nonexistent/tape.jsonl",
	}
	err := validateRecordingPlan(desc)
	if err == nil {
		t.Fatal("expected error for replay mode with missing tape file")
	}
	if !strings.Contains(err.Error(), "replay tape unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRecordingPlanRejectsUnknownMode(t *testing.T) {
	desc := &PreparedRunDescriptor{
		RecordingMode: "telepathy",
	}
	err := validateRecordingPlan(desc)
	if err == nil {
		t.Fatal("expected error for unknown recording_mode")
	}
	if !strings.Contains(err.Error(), "unsupported recording_mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRecordingPlanAcceptsOffRecordReplay(t *testing.T) {
	tapeDir := t.TempDir()
	tapeFile := filepath.Join(tapeDir, "tape.jsonl")
	if err := fs.WriteFileSecure(tapeFile, []byte("{}")); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		mode        string
		tapePath    string
		wantErr     bool
		errContains string
	}{
		{name: "empty mode", mode: "", tapePath: "", wantErr: false},
		{name: "off mode", mode: "off", tapePath: "", wantErr: false},
		{name: "record mode", mode: "record", tapePath: "/some/path", wantErr: false},
		{name: "record mode empty path", mode: "record", tapePath: "", wantErr: false},
		{name: "replay with existing tape", mode: "replay", tapePath: tapeFile, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := &PreparedRunDescriptor{RecordingMode: tt.mode, TapePath: tt.tapePath}
			err := validateRecordingPlan(desc)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr && tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.errContains)
			}
		})
	}
}

func TestPreparedRunExecutorFnLoadsAndValidates(t *testing.T) {
	dir := t.TempDir()
	desc := validDescriptorWithWorkspace(t, dir)
	descPath := filepath.Join(dir, "prepared_run.json")
	if err := desc.Write(descPath); err != nil {
		t.Fatal(err)
	}

	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})
	if err := exec.Execute(context.Background(), desc, io.Discard); err != nil {
		t.Fatalf("unexpected error for valid descriptor: %v", err)
	}

	badDesc := validDescriptorWithWorkspace(t, dir)
	badDesc.RecordingMode = "telepathy"
	badDescPath := filepath.Join(dir, "bad_prepared_run.json")
	if err := badDesc.Write(badDescPath); err != nil {
		t.Fatal(err)
	}
	err := exec.Execute(context.Background(), badDesc, io.Discard)
	if err == nil {
		t.Fatal("expected error for bad recording mode")
	}
	if !strings.Contains(err.Error(), "recording plan") {
		t.Fatalf("error does not mention recording plan: %v", err)
	}
}

func TestWriteCaseReportCreatesValidJSON(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")
	report := CaseReport{
		Name:     "smoke",
		Success:  true,
		Model:    "gemma4:12b",
		Provider: "ollama",
	}

	if err := WriteCaseReport(reportPath, report); err != nil {
		t.Fatalf("WriteCaseReport: %v", err)
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var decoded CaseReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Name != "smoke" {
		t.Fatalf("Name = %q, want %q", decoded.Name, "smoke")
	}
	if !decoded.Success {
		t.Fatal("Success should be true")
	}
}

func TestPreparedRunExecutorFnNoLongerReturnsNotConfigured(t *testing.T) {
	dir := t.TempDir()
	desc := validDescriptorWithWorkspace(t, dir)
	descPath := filepath.Join(dir, "prepared_run.json")
	if err := desc.Write(descPath); err != nil {
		t.Fatal(err)
	}

	exec := (&PreparedRunExecutor{}).WithRunnerOverride(fakeRunner{})
	err := exec.Execute(context.Background(), desc, io.Discard)
	if err != nil && strings.Contains(err.Error(), "not configured") {
		t.Fatalf("executor returned stub error: %v", err)
	}
}

func TestValidateRecordingPlanReplayAcceptsExistingTape(t *testing.T) {
	tapeDir := t.TempDir()
	tapeFile := filepath.Join(tapeDir, "tape.jsonl")
	if err := fs.WriteFileSecure(tapeFile, []byte("{}")); err != nil {
		t.Fatal(err)
	}

	desc := &PreparedRunDescriptor{
		RecordingMode: "replay",
		TapePath:      tapeFile,
	}
	if err := validateRecordingPlan(desc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
