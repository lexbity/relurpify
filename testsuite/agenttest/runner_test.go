//go:build live
// +build live

package agenttest

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/fs"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
	euclosubject "codeburg.org/lexbit/relurpify/testsuite/subjects/euclo"
)

const (
	agenttests                = "agenttests"
	api_ps                    = "/api/ps"
	api_tags                  = "/api/tags"
	application_json          = "application/json"
	artifacts                 = "artifacts"
	assertion                 = "assertion"
	basic_edit_task           = "basic_edit_task"
	content_type              = "Content-Type"
	euclo_code                = "euclo.code"
	euclo_code_testsuite_yaml = "euclo.code.testsuite.yaml"
	kind                      = "kind"
	manifest_model            = "manifest-model"
	mode                      = "mode"
	model                     = "model"
	models                    = "models"
	name                      = "name"
	scope                     = "scope"
	suite_model               = "suite-model"
	testsuite                 = "testsuite"
)

type loadedOllamaServer struct {
	URL    string
	server *http.Server
	ln     net.Listener
}

func (s *loadedOllamaServer) Close() error {
	if s == nil {
		return nil
	}
	if s.server != nil {
		_ = s.server.Close()
	}
	if s.ln != nil {
		return s.ln.Close()
	}
	return nil
}

func newTestHTTPServer(t *testing.T, handler http.Handler) *loadedOllamaServer {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("listen test server: %v", err)
		return nil
	}
	srv := &http.Server{Handler: handler}
	server := &loadedOllamaServer{
		URL:    "http://" + ln.Addr().String(),
		server: srv,
		ln:     ln,
	}
	go func() {
		_ = srv.Serve(ln)
	}()
	return server
}

func newLoadedOllamaServer(t *testing.T, modelName string) *loadedOllamaServer {
	t.Helper()
	return newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case api_tags:
			w.Header().Set(content_type, application_json)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"models":[{"name":"%s","model":"%s","digest":"sha256:test"}]}`, modelName, modelName)))
		case api_ps:
			w.Header().Set(content_type, application_json)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"models":[{"name":"%s","model":"%s","digest":"sha256:test"}]}`, modelName, modelName)))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestParseSetupFileModeDefaultsAndOctal(t *testing.T) {
	mode, err := parseSetupFileMode("")
	if err != nil {
		t.Fatalf("parseSetupFileMode default: %v", err)
	}
	if mode != 0o644 {
		t.Fatalf("expected default 0644, got %#o", mode)
	}

	mode, err = parseSetupFileMode("0755")
	if err != nil {
		t.Fatalf("parseSetupFileMode explicit: %v", err)
	}
	if mode != 0o755 {
		t.Fatalf("expected 0755, got %#o", mode)
	}
}

func TestResolveCaseMaxIterationsPrefersCaseOverride(t *testing.T) {
	got := resolveCaseMaxIterations(RunOptions{MaxIterations: 3}, CaseSpec{
		Overrides: CaseOverrideSpec{MaxIterations: 5},
	})
	if got != 5 {
		t.Fatalf("expected case override 5, got %d", got)
	}

	got = resolveCaseMaxIterations(RunOptions{}, CaseSpec{})
	if got != 8 {
		t.Fatalf("expected default 8, got %d", got)
	}
}

func TestResolveCaseMaxRetries(t *testing.T) {
	if got := resolveCaseMaxRetries(RunOptions{}); got != 3 {
		t.Fatalf("expected default max retries 3, got %d", got)
	}
	if got := resolveCaseMaxRetries(RunOptions{MaxRetries: 5}); got != 5 {
		t.Fatalf("expected explicit max retries 5, got %d", got)
	}
	if got := resolveCaseMaxRetries(RunOptions{MaxRetries: -1}); got != 0 {
		t.Fatalf("expected negative max retries to disable retries, got %d", got)
	}
}

func TestResolveCaseExecutionPrefersCLIThenSuiteThenManifestModel(t *testing.T) {
	layout := newRunCaseLayout(t.TempDir(), smoke, model)
	suite := &Suite{Spec: SuiteSpec{}}

	exec, err := resolveCaseExecution(suite, CaseSpec{Name: smoke}, ModelSpec{Name: suite_model}, manifest_model, RunOptions{ModelOverride: "cli-model"}, layout, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("resolveCaseExecution cli override: %v", err)
	}
	if exec.Model != "cli-model" || exec.ModelSource != "cli_override" {
		t.Fatalf("unexpected cli resolution: %#v", exec)
	}

	exec, err = resolveCaseExecution(suite, CaseSpec{Name: smoke}, ModelSpec{Name: suite_model}, manifest_model, RunOptions{}, layout, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("resolveCaseExecution suite model: %v", err)
	}
	if exec.Model != suite_model || exec.ModelSource != "suite_or_case" {
		t.Fatalf("unexpected suite resolution: %#v", exec)
	}

	exec, err = resolveCaseExecution(suite, CaseSpec{Name: smoke}, ModelSpec{}, manifest_model, RunOptions{}, layout, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("resolveCaseExecution manifest model: %v", err)
	}
	if exec.Model != manifest_model || exec.ModelSource != "manifest" {
		t.Fatalf("unexpected manifest resolution: %#v", exec)
	}
}

func TestResolveCaseExecutionFailsWithoutResolvedModel(t *testing.T) {
	layout := newRunCaseLayout(t.TempDir(), smoke, model)
	_, err := resolveCaseExecution(&Suite{Spec: SuiteSpec{}}, CaseSpec{Name: smoke}, ModelSpec{}, "", RunOptions{}, layout, t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("expected missing model to fail")
	}
}

func TestResolveCaseExecutionReplayRequiresTape(t *testing.T) {
	layout := newRunCaseLayout(t.TempDir(), smoke, model)
	suite := &Suite{
		Spec: SuiteSpec{
			Recording: RecordingSpec{Mode: "replay", Tape: "missing.jsonl"},
		},
	}
	_, err := resolveCaseExecution(suite, CaseSpec{Name: smoke}, ModelSpec{Name: suite_model}, manifest_model, RunOptions{}, layout, t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("expected replay without tape to fail")
	}
}

func TestResolveCaseExecutionUsesGoldenTapeForReplayStrategy(t *testing.T) {
	workspace := t.TempDir()
	suitePath := filepath.Join(workspace, testsuite, agenttests, euclo_code_testsuite_yaml)
	goldenDir := filepath.Join(workspace, testsuite, agenttests, "tapes", euclo_code)
	if err := fs.MkdirAllSecure(goldenDir); err != nil {
		t.Fatal(err)
	}
	goldenPath := filepath.Join(goldenDir, "basic_edit_task__qwen2_5_coder_14b.tape.jsonl")
	if err := fs.WriteFileSecure(goldenPath, []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	layout := newRunCaseLayout(t.TempDir(), basic_edit_task, qwen2_5_coder_14b)
	suite := &Suite{
		SourcePath: suitePath,
		Metadata:   SuiteMeta{Name: euclo_code},
		Spec: SuiteSpec{
			Recording: RecordingSpec{Strategy: "replay-if-golden"},
		},
	}

	exec, err := resolveCaseExecution(suite, CaseSpec{Name: basic_edit_task}, ModelSpec{Name: qwen2_5_coder_14b}, manifest_model, RunOptions{}, layout, workspace, workspace)
	if err != nil {
		t.Fatalf("resolveCaseExecution replay strategy: %v", err)
	}
	if exec.RecordingMode != "replay" {
		t.Fatalf("expected replay mode, got %q", exec.RecordingMode)
	}
	if exec.TapePath != goldenPath {
		t.Fatalf("expected golden tape path %q, got %q", goldenPath, exec.TapePath)
	}
}

func TestResolveCaseExecutionReplayIfGoldenFallsBackToLiveWhenMissing(t *testing.T) {
	layout := newRunCaseLayout(t.TempDir(), basic_edit_task, qwen2_5_coder_14b)
	suite := &Suite{
		SourcePath: filepath.Join(t.TempDir(), testsuite, agenttests, euclo_code_testsuite_yaml),
		Metadata:   SuiteMeta{Name: euclo_code},
		Spec: SuiteSpec{
			Recording: RecordingSpec{Strategy: "replay-if-golden"},
		},
	}

	exec, err := resolveCaseExecution(suite, CaseSpec{Name: basic_edit_task}, ModelSpec{Name: qwen2_5_coder_14b}, manifest_model, RunOptions{}, layout, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("resolveCaseExecution replay-if-golden fallback: %v", err)
	}
	if exec.RecordingMode != "off" {
		t.Fatalf("expected live/off mode, got %q", exec.RecordingMode)
	}
	if exec.TapePath != "" {
		t.Fatalf("expected no tape path for live fallback, got %q", exec.TapePath)
	}
}

func TestResolveCaseExecutionReplayOnlyFailsWithoutGoldenTape(t *testing.T) {
	layout := newRunCaseLayout(t.TempDir(), basic_edit_task, qwen2_5_coder_14b)
	suite := &Suite{
		SourcePath: filepath.Join(t.TempDir(), testsuite, agenttests, euclo_code_testsuite_yaml),
		Metadata:   SuiteMeta{Name: euclo_code},
		Spec: SuiteSpec{
			Recording: RecordingSpec{Strategy: "replay-only"},
		},
	}

	_, err := resolveCaseExecution(suite, CaseSpec{Name: basic_edit_task}, ModelSpec{Name: qwen2_5_coder_14b}, manifest_model, RunOptions{}, layout, t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("expected replay-only without golden tape to fail")
	}
}

func TestExpandSuiteModelMatrixUsesDeterministicOrder(t *testing.T) {
	models := []ModelSpec{{Name: "m1"}, {Name: "m2"}}
	providers := []ProviderSpec{{Name: "p1"}, {Name: "p2"}}

	got := ExpandSuiteModelMatrix(models, providers, "provider-first")
	want := []string{"p1:m1", "p1:m2", "p2:m1", "p2:m2"}
	if len(got) != len(want) {
		t.Fatalf("unexpected matrix length %d, want %d", len(got), len(want))
	}
	for i, row := range got {
		if gotID := row.Provider + ":" + row.Name; gotID != want[i] {
			t.Fatalf("provider-first row[%d] = %q, want %q", i, gotID, want[i])
		}
	}

	got = ExpandSuiteModelMatrix(models, providers, "model-first")
	want = []string{"p1:m1", "p2:m1", "p1:m2", "p2:m2"}
	for i, row := range got {
		if gotID := row.Provider + ":" + row.Name; gotID != want[i] {
			t.Fatalf("model-first row[%d] = %q, want %q", i, gotID, want[i])
		}
	}
}

func TestRunnerPreflightSuiteChecksLoadedModels(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := filepath.Join(workspace, relurpify_cfg, agent_yaml)
	if err := fs.MkdirAllSecure(filepath.Dir(manifestPath)); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	manifestData := `schema: relurpify/agent/v1
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: coding
spec:
  image: ghcr.io/lexcodex/relurpify/runtime:latest
  runtime: gvisor
  defaults:
    permissions:
      filesystem:
        - action: fs:read
          path: /tmp/**
          justification: Read workspace
  agent:
    implementation: coding
    mode: primary
    model:
      provider: ollama
      name: gemma4:12b
`
	if err := fs.WriteFileSecure(manifestPath, []byte(manifestData)); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	server := newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case api_tags, api_ps:
			w.Header().Set(content_type, application_json)
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen2_5_coder_14b"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	suite := &Suite{
		SourcePath: filepath.Join(workspace, testsuite, agenttests, "coding.testsuite.yaml"),
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.yaml",
			Workspace: WorkspaceSpec{Strategy: "derived"},
			Models:    []ModelSpec{{Name: qwen2_5_coder_14b, Endpoint: server.URL}},
			Cases: []CaseSpec{{
				Name:   smoke,
				Prompt: hello,
			}},
		},
	}

	if err := (&Runner{}).preflightSuite(context.Background(), suite, RunOptions{}, workspace, suite.Spec.Models); err != nil {
		t.Fatalf("preflightSuite: %v", err)
	}
}

func TestRunnerPreflightSuiteFailsWhenModelNotLoaded(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := filepath.Join(workspace, relurpify_cfg, agent_yaml)
	if err := fs.MkdirAllSecure(filepath.Dir(manifestPath)); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	manifestData := `schema: relurpify/agent/v1
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: coding
spec:
  image: ghcr.io/lexcodex/relurpify/runtime:latest
  runtime: gvisor
  defaults:
    permissions:
      filesystem:
        - action: fs:read
          path: /tmp/**
          justification: Read workspace
  agent:
    implementation: coding
    mode: primary
    model:
      provider: ollama
      name: gemma4:12b
`
	if err := fs.WriteFileSecure(manifestPath, []byte(manifestData)); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	server := newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case api_tags:
			w.Header().Set(content_type, application_json)
			_, _ = w.Write([]byte(`{"models":[{"name":"other-model"}]}`))
		case api_ps:
			w.Header().Set(content_type, application_json)
			_, _ = w.Write([]byte(`{"models":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	suite := &Suite{
		SourcePath: filepath.Join(workspace, testsuite, agenttests, "coding.testsuite.yaml"),
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.yaml",
			Workspace: WorkspaceSpec{Strategy: "derived"},
			Models:    []ModelSpec{{Name: qwen2_5_coder_14b, Endpoint: server.URL}},
			Cases: []CaseSpec{{
				Name:   smoke,
				Prompt: hello,
			}},
		},
	}

	err := (&Runner{}).preflightSuite(context.Background(), suite, RunOptions{}, workspace, suite.Spec.Models)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected preflight model-not-found error, got %v", err)
	}
}

func TestClassifyFailure(t *testing.T) {
	if got := classifyFailure(nil); got != "" {
		t.Fatalf("expected empty classification for nil error, got %q", got)
	}
	if got := classifyFailure(assertionErr("mismatch for interaction 3")); got != assertion {
		t.Fatalf("expected tape mismatch classification to be assertion, got %q", got)
	}
	if got := classifyFailure(assertionErr("context deadline exceeded")); got != infra {
		t.Fatalf("expected infra classification, got %q", got)
	}
	if got := classifyFailure(assertionErr("permission denied: /etc/passwd")); got != security {
		t.Fatalf("expected security classification, got %q", got)
	}
}

func TestCountTokenUsage(t *testing.T) {
	usage := CountTokenUsage([]telemetry.Event{{
		Type: telemetry.EventLLMResponse,
		Metadata: map[string]any{
			"usage": map[string]any{
				"prompt_tokens":     11.0,
				"completion_tokens": 7.0,
			},
		},
	}, {
		Type: telemetry.EventLLMResponse,
		Metadata: map[string]any{
			"usage": map[string]any{
				"prompt_tokens":     5.0,
				"completion_tokens": 3.0,
				"total_tokens":      8.0,
			},
		},
	}})
	if usage.PromptTokens != 16 || usage.CompletionTokens != 10 || usage.TotalTokens != 26 || usage.LLMCalls != 2 {
		t.Fatalf("unexpected token usage: %+v", usage)
	}
}

func TestRunSuiteAggregatesCaseCounts(t *testing.T) {
	report := &SuiteReport{
		Cases: []CaseReport{
			{Success: true},
			{Success: false, FailureKind: infra},
			{Skipped: true, Success: true},
			{Success: false, FailureKind: assertion},
		},
	}
	for _, c := range report.Cases {
		switch {
		case c.Skipped:
			report.SkippedCases++
		case c.Success:
			report.PassedCases++
		default:
			report.FailedCases++
			if c.FailureKind == infra {
				report.InfraFailures++
			} else {
				report.AssertFailures++
			}
		}
	}
	if report.PassedCases != 1 || report.FailedCases != 2 || report.SkippedCases != 1 || report.InfraFailures != 1 || report.AssertFailures != 1 {
		t.Fatalf("unexpected aggregate counts: %+v", report)
	}
}

type assertionErr string

func (e assertionErr) Error() string { return string(e) }

func TestNewRunCaseLayoutUsesStructuredRunSubdirectories(t *testing.T) {
	runRoot := filepath.Join("/tmp", run1)
	layout := newRunCaseLayout(runRoot, "Write Docs", "llama3.2")

	caseKey := "Write_Docs__llama3_2"
	if got := layout.ArtifactsDir; got != filepath.Join(runRoot, artifacts, caseKey) {
		t.Fatalf("ArtifactsDir = %q", got)
	}
	if got := layout.TmpDir; got != filepath.Join(runRoot, "tmp", caseKey) {
		t.Fatalf("TmpDir = %q", got)
	}
	if got := layout.WorkspaceDir; got != filepath.Join(runRoot, "tmp", caseKey, "workspace") {
		t.Fatalf("WorkspaceDir = %q", got)
	}
	if got := layout.LogPath; got != filepath.Join(runRoot, "logs", caseKey+".log") {
		t.Fatalf("LogPath = %q", got)
	}
	if got := layout.TelemetryPath; got != filepath.Join(runRoot, "telemetry", caseKey+".jsonl") {
		t.Fatalf("TelemetryPath = %q", got)
	}
	if got := layout.TapePath; got != filepath.Join(runRoot, artifacts, caseKey, "tape.jsonl") {
		t.Fatalf("TapePath = %q", got)
	}
	if got := layout.InteractionTapePath; got != filepath.Join(runRoot, artifacts, caseKey, "interaction.tape.jsonl") {
		t.Fatalf("InteractionTapePath = %q", got)
	}
}

func TestMarshalInteractionRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interaction.tape.jsonl")
	if err := euclosubject.WriteInteractionTape(path, map[string]any{
		"euclo.interaction_records": []any{
			map[string]any{kind: "proposal", "phase": scope},
			map[string]any{kind: "question", "phase": "clarify"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], `"kind":"proposal"`) {
		t.Fatalf("unexpected first line %q", lines[0])
	}
}
