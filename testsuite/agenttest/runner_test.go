package agenttest

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	euclosubject "codeburg.org/lexbit/relurpify/testsuite/subjects/euclo"
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
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"models":[{"name":%q,"model":%q,"digest":"sha256:test"}]}`, modelName, modelName)))
		case "/api/ps":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"models":[{"name":%q,"model":%q,"digest":"sha256:test"}]}`, modelName, modelName)))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestFallbackManifestPath(t *testing.T) {
	workspace := t.TempDir()
	manifest := manifest.New(workspace).ManifestFile()
	if err := os.MkdirAll(filepath.Dir(manifest), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(manifest, []byte("test"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got := fallbackManifestPath(filepath.Join(workspace, "testsuite", "agent.manifest.yaml"), workspace)
	if got != manifest {
		t.Fatalf("expected %s, got %s", manifest, got)
	}
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
	layout := newRunCaseLayout(t.TempDir(), "smoke", "model")
	suite := &Suite{Spec: SuiteSpec{}}

	exec, err := resolveCaseExecution(suite, CaseSpec{Name: "smoke"}, ModelSpec{Name: "suite-model"}, "manifest-model", RunOptions{ModelOverride: "cli-model"}, layout, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("resolveCaseExecution cli override: %v", err)
	}
	if exec.Model != "cli-model" || exec.ModelSource != "cli_override" {
		t.Fatalf("unexpected cli resolution: %#v", exec)
	}

	exec, err = resolveCaseExecution(suite, CaseSpec{Name: "smoke"}, ModelSpec{Name: "suite-model"}, "manifest-model", RunOptions{}, layout, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("resolveCaseExecution suite model: %v", err)
	}
	if exec.Model != "suite-model" || exec.ModelSource != "suite_or_case" {
		t.Fatalf("unexpected suite resolution: %#v", exec)
	}

	exec, err = resolveCaseExecution(suite, CaseSpec{Name: "smoke"}, ModelSpec{}, "manifest-model", RunOptions{}, layout, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("resolveCaseExecution manifest model: %v", err)
	}
	if exec.Model != "manifest-model" || exec.ModelSource != "manifest" {
		t.Fatalf("unexpected manifest resolution: %#v", exec)
	}
}

func TestResolveCaseModelProfileUsesWorkspaceRegistry(t *testing.T) {
	workspace := t.TempDir()
	profilesDir := filepath.Join(workspace, "relurpify_cfg", "model_profiles")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "default.yaml"), []byte(`pattern: "*"
repair:
  strategy: heuristic-only
  max_attempts: 0
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "gemma4.yaml"), []byte(`pattern: "gemma4*"
tool_calling:
  native_api: true
  max_tools_per_call: 2
repair:
  strategy: llm
  max_attempts: 1
`), 0o644); err != nil {
		t.Fatal(err)
	}

	provenance, profile, err := resolveCaseModelProfile(workspace, resolvedCaseExecution{
		Provider: "ollama",
		Model:    "gemma4:e4b",
	})
	if err != nil {
		t.Fatalf("resolveCaseModelProfile: %v", err)
	}
	if provenance == nil || profile == nil {
		t.Fatal("expected model profile provenance and profile")
	}
	if provenance.MatchKind != "glob" {
		t.Fatalf("expected glob match kind, got %q", provenance.MatchKind)
	}
	if provenance.ProfileSource != filepath.ToSlash("relurpify_cfg/model_profiles/gemma4.yaml") {
		t.Fatalf("unexpected profile source: %q", provenance.ProfileSource)
	}
	if profile.Repair.Strategy != "llm" || !profile.ToolCalling.NativeAPI || profile.ToolCalling.MaxToolsPerCall != 2 {
		t.Fatalf("unexpected resolved profile: %#v", profile)
	}
}

func TestResolveCaseExecutionFailsWithoutResolvedModel(t *testing.T) {
	layout := newRunCaseLayout(t.TempDir(), "smoke", "model")
	_, err := resolveCaseExecution(&Suite{Spec: SuiteSpec{}}, CaseSpec{Name: "smoke"}, ModelSpec{}, "", RunOptions{}, layout, t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("expected missing model to fail")
	}
}

func TestResolveCaseExecutionReplayRequiresTape(t *testing.T) {
	layout := newRunCaseLayout(t.TempDir(), "smoke", "model")
	suite := &Suite{
		Spec: SuiteSpec{
			Recording: RecordingSpec{Mode: "replay", Tape: "missing.jsonl"},
		},
	}
	_, err := resolveCaseExecution(suite, CaseSpec{Name: "smoke"}, ModelSpec{Name: "suite-model"}, "manifest-model", RunOptions{}, layout, t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("expected replay without tape to fail")
	}
}

func TestResolveCaseExecutionUsesGoldenTapeForReplayStrategy(t *testing.T) {
	workspace := t.TempDir()
	suitePath := filepath.Join(workspace, "testsuite", "agenttests", "euclo.code.testsuite.yaml")
	goldenDir := filepath.Join(workspace, "testsuite", "agenttests", "tapes", "euclo.code")
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	goldenPath := filepath.Join(goldenDir, "basic_edit_task__qwen2_5_coder_14b.tape.jsonl")
	if err := os.WriteFile(goldenPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	layout := newRunCaseLayout(t.TempDir(), "basic_edit_task", "qwen2.5-coder:14b")
	suite := &Suite{
		SourcePath: suitePath,
		Metadata:   SuiteMeta{Name: "euclo.code"},
		Spec: SuiteSpec{
			Recording: RecordingSpec{Strategy: "replay-if-golden"},
		},
	}

	exec, err := resolveCaseExecution(suite, CaseSpec{Name: "basic_edit_task"}, ModelSpec{Name: "qwen2.5-coder:14b"}, "manifest-model", RunOptions{}, layout, workspace, workspace)
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
	layout := newRunCaseLayout(t.TempDir(), "basic_edit_task", "qwen2.5-coder:14b")
	suite := &Suite{
		SourcePath: filepath.Join(t.TempDir(), "testsuite", "agenttests", "euclo.code.testsuite.yaml"),
		Metadata:   SuiteMeta{Name: "euclo.code"},
		Spec: SuiteSpec{
			Recording: RecordingSpec{Strategy: "replay-if-golden"},
		},
	}

	exec, err := resolveCaseExecution(suite, CaseSpec{Name: "basic_edit_task"}, ModelSpec{Name: "qwen2.5-coder:14b"}, "manifest-model", RunOptions{}, layout, t.TempDir(), t.TempDir())
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
	layout := newRunCaseLayout(t.TempDir(), "basic_edit_task", "qwen2.5-coder:14b")
	suite := &Suite{
		SourcePath: filepath.Join(t.TempDir(), "testsuite", "agenttests", "euclo.code.testsuite.yaml"),
		Metadata:   SuiteMeta{Name: "euclo.code"},
		Spec: SuiteSpec{
			Recording: RecordingSpec{Strategy: "replay-only"},
		},
	}

	_, err := resolveCaseExecution(suite, CaseSpec{Name: "basic_edit_task"}, ModelSpec{Name: "qwen2.5-coder:14b"}, "manifest-model", RunOptions{}, layout, t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("expected replay-only without golden tape to fail")
	}
}

func TestExpandSuiteModelMatrixUsesDeterministicOrder(t *testing.T) {
	models := []ModelSpec{{Name: "m1"}, {Name: "m2"}}
	providers := []ProviderSpec{{Name: "p1"}, {Name: "p2"}}

	got := expandSuiteModelMatrix(models, providers, "provider-first")
	want := []string{"p1:m1", "p1:m2", "p2:m1", "p2:m2"}
	if len(got) != len(want) {
		t.Fatalf("unexpected matrix length %d, want %d", len(got), len(want))
	}
	for i, row := range got {
		if gotID := row.Provider + ":" + row.Name; gotID != want[i] {
			t.Fatalf("provider-first row[%d] = %q, want %q", i, gotID, want[i])
		}
	}

	got = expandSuiteModelMatrix(models, providers, "model-first")
	want = []string{"p1:m1", "p2:m1", "p1:m2", "p2:m2"}
	for i, row := range got {
		if gotID := row.Provider + ":" + row.Name; gotID != want[i] {
			t.Fatalf("model-first row[%d] = %q, want %q", i, gotID, want[i])
		}
	}
}

func TestProviderProvenanceForExecution(t *testing.T) {
	prov := providerProvenanceForExecution(resolvedCaseExecution{
		Provider:              "ollama",
		Endpoint:              "http://localhost:11434",
		ProviderResetStrategy: "model",
		ProviderResetBetween:  true,
	})
	if prov == nil {
		t.Fatal("expected provider provenance")
	}
	if prov.Provider != "ollama" || prov.ResetStrategy != "model" || !prov.ResetBetween {
		t.Fatalf("unexpected provenance: %+v", prov)
	}
}

func TestResolveCaseTimeout(t *testing.T) {
	got, err := resolveCaseTimeout(RunOptions{Timeout: 30 * time.Second}, nil, CaseSpec{Timeout: "2m"})
	if err != nil {
		t.Fatalf("resolveCaseTimeout case override: %v", err)
	}
	if got != 2*time.Minute {
		t.Fatalf("expected 2m timeout, got %s", got)
	}

	got, err = resolveCaseTimeout(RunOptions{Timeout: 30 * time.Second}, nil, CaseSpec{})
	if err != nil {
		t.Fatalf("resolveCaseTimeout global: %v", err)
	}
	if got != 30*time.Second {
		t.Fatalf("expected 30s timeout, got %s", got)
	}

	got, err = resolveCaseTimeout(RunOptions{Timeout: 30 * time.Second}, &Suite{
		Spec: SuiteSpec{
			Execution: SuiteExecutionSpec{Timeout: "90s"},
		},
	}, CaseSpec{})
	if err != nil {
		t.Fatalf("resolveCaseTimeout suite timeout: %v", err)
	}
	if got != 90*time.Second {
		t.Fatalf("expected suite timeout 90s, got %s", got)
	}

	if _, err := resolveCaseTimeout(RunOptions{}, nil, CaseSpec{Timeout: "nope"}); err == nil {
		t.Fatal("expected invalid case timeout to fail")
	}
}

func TestResolveBootstrapTimeout(t *testing.T) {
	if got := resolveBootstrapTimeout(RunOptions{BootstrapTimeout: 5 * time.Second}, CaseSpec{}); got != 5*time.Second {
		t.Fatalf("expected 5s bootstrap timeout, got %s", got)
	}
	if got := resolveBootstrapTimeout(RunOptions{}, CaseSpec{}); got != 30*time.Second {
		t.Fatalf("expected default bootstrap timeout, got %s", got)
	}
	if got := resolveBootstrapTimeout(RunOptions{BootstrapTimeout: 5 * time.Second}, CaseSpec{
		Overrides: CaseOverrideSpec{BootstrapTimeout: "45s"},
	}); got != 45*time.Second {
		t.Fatalf("expected case bootstrap timeout override 45s, got %s", got)
	}
}

func TestApplySetupGitInitCreatesCommittedBaseline(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	workspace := t.TempDir()
	cleanup, err := applySetup(workspace, SetupSpec{
		GitInit: true,
		Files: []SetupFileSpec{{
			Path:    "testsuite/agenttest_fixtures/hello.txt",
			Content: "hello\n",
		}},
	}, false, nil)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatalf("applySetup: %v", err)
	}

	cmd := exec.Command("git", "status", "--short")
	cmd.Dir = workspace
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v (%s)", err, string(out))
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("expected clean baseline git status, got %q", strings.TrimSpace(string(out)))
	}
}

func TestShouldRestrictAllowedCapabilitiesForCase(t *testing.T) {
	if !shouldRestrictAllowedCapabilitiesForCase(CaseSpec{
		TaskType: "analysis",
	}) {
		t.Fatal("expected analysis case to restrict to explicit allowed capabilities")
	}
	if !shouldRestrictAllowedCapabilitiesForCase(CaseSpec{
		TaskType: "code-modification",
		Context:  map[string]any{"mode": "debug"},
	}) {
		t.Fatal("expected debug case to restrict to explicit allowed capabilities")
	}
	if shouldRestrictAllowedCapabilitiesForCase(CaseSpec{
		TaskType: "code-modification",
		Context:  map[string]any{"mode": "docs"},
	}) {
		t.Fatal("expected docs edit case to keep default capabilities merged")
	}
}

func TestSeedWorkflowRetrievalStateForCase(t *testing.T) {
	state := contextdata.NewEnvelope("task-1", "session-1")
	task := &core.Task{
		Instruction: "Summarize README.md",
		Context: map[string]any{
			"mode":        "architect",
			"workflow_id": "wf-1",
		},
	}
	c := CaseSpec{
		Setup: SetupSpec{
			Workflows: []WorkflowSeedSpec{{
				Workflow: WorkflowRecordSeedSpec{WorkflowID: "wf-1"},
				Knowledge: []WorkflowKnowledgeSeedSpec{{
					RecordID: "k-1",
					Content:  "Use retrieval-backed planning context.",
				}},
			}},
		},
	}

	seedWorkflowRetrievalStateForCase(state, task, c)

	if got := fmt.Sprint(task.Context["workflow_retrieval"]); !strings.Contains(got, "retrieval-backed") {
		t.Fatalf("expected task workflow retrieval payload, got %q", got)
	}
	raw, ok := state.GetWorkingValue("planner.workflow_retrieval")
	if !ok {
		t.Fatal("expected planner.workflow_retrieval state")
	}
	if got := fmt.Sprint(raw); !strings.Contains(got, "retrieval-backed") {
		t.Fatalf("expected planner workflow retrieval state, got %q", got)
	}
}

func TestSeedWorkflowRetrievalStateForCaseSeedsCompiledPlanFromWorkflowKnowledge(t *testing.T) {
	state := contextdata.NewEnvelope("task-2", "session-1")
	task := &core.Task{
		Instruction: "Execute the compiled plan",
		Context: map[string]any{
			"mode":        "planning",
			"workflow_id": "wf-compiled",
		},
	}
	c := CaseSpec{
		Setup: SetupSpec{
			Workflows: []WorkflowSeedSpec{{
				Workflow: WorkflowRecordSeedSpec{WorkflowID: "wf-compiled"},
				Knowledge: []WorkflowKnowledgeSeedSpec{{
					RecordID: "k-plan",
					Title:    "Compiled plan",
					Content:  "Plan: update testsuite/fixtures/rapid_arch_exec/slug.go so NormalizeSlug trims whitespace and lowercases the slug before returning it.",
				}},
			}},
		},
	}

	seedWorkflowRetrievalStateForCase(state, task, c)

	raw, ok := state.GetWorkingValue("pipeline.plan")
	if !ok {
		t.Fatal("expected pipeline.plan to be seeded")
	}
	plan, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected seeded pipeline.plan payload, got %T", raw)
	}
	steps, ok := plan["steps"].([]map[string]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("expected a single seeded plan step, got %#v", plan["steps"])
	}
	scope, ok := steps[0]["scope"].([]string)
	if !ok || len(scope) != 1 || scope[0] != "testsuite/fixtures/rapid_arch_exec/slug.go" {
		t.Fatalf("expected seeded scope from workflow knowledge, got %#v", steps[0]["scope"])
	}
	got, ok := state.GetWorkingValue("pipeline.workflow_retrieval")
	if !ok {
		t.Fatal("expected pipeline.workflow_retrieval to be seeded")
	}
	if gotStr := fmt.Sprint(got); !strings.Contains(gotStr, "compiled plan") {
		t.Fatalf("expected workflow retrieval payload to mention compiled plan, got %q", gotStr)
	}
}

func TestRunnerPreflightSuiteChecksLoadedModels(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agent.manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	manifestData := `apiVersion: relurpify/v1alpha1
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
      name: qwen2.5-coder:14b
`
	if err := os.WriteFile(manifestPath, []byte(manifestData), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	server := newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[]}`))
		case "/api/ps":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen2.5-coder:14b"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	suite := &Suite{
		SourcePath: filepath.Join(workspace, "testsuite", "agenttests", "coding.testsuite.yaml"),
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Workspace: WorkspaceSpec{Strategy: "derived"},
			Models:    []ModelSpec{{Name: "qwen2.5-coder:14b", Endpoint: server.URL}},
			Cases: []CaseSpec{{
				Name:   "smoke",
				Prompt: "hello",
			}},
		},
	}

	if err := (&Runner{}).preflightSuite(suite, RunOptions{}, workspace, suite.Spec.Models); err != nil {
		t.Fatalf("preflightSuite: %v", err)
	}
}

func TestRunnerPreflightSuiteFailsWhenModelNotLoaded(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agent.manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	manifestData := `apiVersion: relurpify/v1alpha1
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
      name: qwen2.5-coder:14b
`
	if err := os.WriteFile(manifestPath, []byte(manifestData), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	server := newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[]}`))
		case "/api/ps":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	suite := &Suite{
		SourcePath: filepath.Join(workspace, "testsuite", "agenttests", "coding.testsuite.yaml"),
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Workspace: WorkspaceSpec{Strategy: "derived"},
			Models:    []ModelSpec{{Name: "qwen2.5-coder:14b", Endpoint: server.URL}},
			Cases: []CaseSpec{{
				Name:   "smoke",
				Prompt: "hello",
			}},
		},
	}

	err := (&Runner{}).preflightSuite(suite, RunOptions{}, workspace, suite.Spec.Models)
	if err == nil || !strings.Contains(err.Error(), "not loaded") {
		t.Fatalf("expected preflight model-not-loaded error, got %v", err)
	}
}

func TestClassifyCaseFailure(t *testing.T) {
	if got := classifyCaseFailure(nil, `output missing "done"`); got != "assertion" {
		t.Fatalf("expected assertion classification, got %q", got)
	}
	if got := classifyCaseFailure(assertionErr("mismatch for interaction 3"), "mismatch for interaction 3"); got != "assertion" {
		t.Fatalf("expected tape mismatch classification to be assertion, got %q", got)
	}
	if got := classifyCaseFailure(assertionErr("connection refused"), "connection refused"); got != "infra" {
		t.Fatalf("expected infra classification, got %q", got)
	}
	if got := classifyCaseFailure(assertionErr("agent returned unsuccessful result"), "agent returned unsuccessful result"); got != "agent" {
		t.Fatalf("expected agent classification, got %q", got)
	}
}

func TestCountTokenUsage(t *testing.T) {
	usage := CountTokenUsage([]core.Event{{
		Type: core.EventLLMResponse,
		Metadata: map[string]any{
			"usage": map[string]any{
				"prompt_tokens":     11.0,
				"completion_tokens": 7.0,
			},
		},
	}, {
		Type: core.EventLLMResponse,
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

func TestLookupBackendModelProvenance(t *testing.T) {
	server := newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[]}`))
		case "/api/ps":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen2.5-coder:14b-q8_0","model":"qwen2.5-coder:14b","digest":"sha256:abc123","details":{"quantization_level":"Q8_0"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provenance, err := lookupBackendModelProvenance(server.URL, "qwen2.5-coder:14b")
	if err != nil {
		t.Fatalf("lookupBackendModelProvenance: %v", err)
	}
	if provenance == nil || provenance.Digest != "sha256:abc123" || provenance.LoadedName != "qwen2.5-coder:14b-q8_0" {
		t.Fatalf("unexpected provenance: %+v", provenance)
	}
}

func TestRunSuiteAggregatesCaseCounts(t *testing.T) {
	report := &SuiteReport{
		Cases: []CaseReport{
			{Success: true},
			{Success: false, FailureKind: "infra"},
			{Skipped: true, Success: true},
			{Success: false, FailureKind: "assertion"},
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
			if c.FailureKind == "infra" {
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

func TestIncludeExpectedChangedFilesRestoresIgnoredExpectation(t *testing.T) {
	workflowStateRel := filepath.ToSlash(filepath.Join(manifest.DirName, "sessions", "workflow_state.db"))
	before := &WorkspaceSnapshot{
		Files: map[string]string{
			workflowStateRel: "before",
		},
	}
	after := &WorkspaceSnapshot{
		Files: map[string]string{
			workflowStateRel: "after",
		},
	}

	changed := includeExpectedChangedFiles(nil, before, after, []string{workflowStateRel})

	if len(changed) != 1 || changed[0] != workflowStateRel {
		t.Fatalf("expected workflow_state.db to be restored, got %#v", changed)
	}
}

func TestNewRunCaseLayoutUsesStructuredRunSubdirectories(t *testing.T) {
	runRoot := filepath.Join("/tmp", "run-1")
	layout := newRunCaseLayout(runRoot, "Write Docs", "llama3.2")

	caseKey := "Write_Docs__llama3_2"
	if got := layout.ArtifactsDir; got != filepath.Join(runRoot, "artifacts", caseKey) {
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
	if got := layout.TapePath; got != filepath.Join(runRoot, "artifacts", caseKey, "tape.jsonl") {
		t.Fatalf("TapePath = %q", got)
	}
	if got := layout.InteractionTapePath; got != filepath.Join(runRoot, "artifacts", caseKey, "interaction.tape.jsonl") {
		t.Fatalf("InteractionTapePath = %q", got)
	}
}

func TestMarshalInteractionRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interaction.tape.jsonl")
	if err := euclosubject.WriteInteractionTape(path, map[string]any{
		"euclo.interaction_records": []any{
			map[string]any{"kind": "proposal", "phase": "scope"},
			map[string]any{"kind": "question", "phase": "clarify"},
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

func TestRunCaseFailsWhenBootstrapExceedsTimeout(t *testing.T) {
	workspace := t.TempDir()
	ollama := newLoadedOllamaServer(t, "qwen2.5-coder:14b")
	defer ollama.Close()
	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agent.manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	manifestData := `apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: coding
spec:
  image: relurpify:test
  runtime: gvisor
  permissions:
    filesystem:
      - action: fs:read
        path: README.md
  resources:
    limits:
      cpu: "1"
      memory: "1Gi"
      disk_io: "1Gi"
  agent:
    implementation: coding
    mode: primary
    model:
      provider: ollama
      name: qwen2.5-coder:14b
`
	if err := os.WriteFile(manifestPath, []byte(manifestData), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	suite := &Suite{
		SourcePath: filepath.Join(workspace, "testsuite", "agenttests", "coding.testsuite.yaml"),
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata:   SuiteMeta{Name: "coding", Tier: "smoke"},
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Workspace: WorkspaceSpec{Strategy: "derived"},
			Models: []ModelSpec{{
				Name:     "qwen2.5-coder:14b",
				Endpoint: ollama.URL,
			}},
			Cases: []CaseSpec{{
				Name:   "bootstrap_timeout",
				Prompt: "Summarize README.md in 5 bullets.",
			}},
		},
	}
	if err := suite.Validate(); err != nil {
		t.Fatalf("suite.Validate: %v", err)
	}

	origBootstrap := bootstrapAgentRuntime
	bootstrapAgentRuntime = func(_ string, opts ayenitd.AgentBootstrapOptions) (*ayenitd.BootstrappedAgentRuntime, error) {
		<-opts.Context.Done()
		return nil, opts.Context.Err()
	}
	defer func() { bootstrapAgentRuntime = origBootstrap }()

	report := (&Runner{}).runCase(
		context.Background(),
		suite,
		suite.Spec.Cases[0],
		suite.Spec.Models[0],
		RunOptions{TargetWorkspace: workspace, OutputDir: t.TempDir(), Timeout: 20 * time.Millisecond, BootstrapTimeout: 20 * time.Millisecond},
		workspace,
		t.TempDir(),
	)

	if report.Success {
		t.Fatal("expected bootstrap timeout to fail case")
	}
	if report.FailureKind != "infra" {
		t.Fatalf("expected infra failure, got %q", report.FailureKind)
	}
	if !strings.Contains(report.Error, context.DeadlineExceeded.Error()) {
		t.Fatalf("expected deadline exceeded error, got %q", report.Error)
	}
}

func TestRunCaseUsesCaseBootstrapTimeoutOverride(t *testing.T) {
	workspace := t.TempDir()
	ollama := newLoadedOllamaServer(t, "qwen2.5-coder:14b")
	defer ollama.Close()
	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agent.manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	manifestData := `apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: coding
spec:
  image: relurpify:test
  runtime: gvisor
  permissions:
    filesystem:
      - action: fs:read
        path: README.md
  resources:
    limits:
      cpu: "1"
      memory: "1Gi"
      disk_io: "1Gi"
  agent:
    implementation: coding
    mode: primary
    model:
      provider: ollama
      name: qwen2.5-coder:14b
`
	if err := os.WriteFile(manifestPath, []byte(manifestData), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	suite := &Suite{
		SourcePath: filepath.Join(workspace, "testsuite", "agenttests", "coding.testsuite.yaml"),
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata:   SuiteMeta{Name: "coding", Tier: "smoke"},
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Workspace: WorkspaceSpec{Strategy: "derived"},
			Models: []ModelSpec{{
				Name:     "qwen2.5-coder:14b",
				Endpoint: ollama.URL,
			}},
			Cases: []CaseSpec{{
				Name:   "bootstrap_timeout_override",
				Prompt: "Summarize README.md in 5 bullets.",
				Overrides: CaseOverrideSpec{
					BootstrapTimeout: "80ms",
				},
			}},
		},
	}
	if err := suite.Validate(); err != nil {
		t.Fatalf("suite.Validate: %v", err)
	}

	origBootstrap := bootstrapAgentRuntime
	bootstrapAgentRuntime = func(_ string, opts ayenitd.AgentBootstrapOptions) (*ayenitd.BootstrappedAgentRuntime, error) {
		time.Sleep(40 * time.Millisecond)
		<-opts.Context.Done()
		return nil, opts.Context.Err()
	}
	defer func() { bootstrapAgentRuntime = origBootstrap }()

	report := (&Runner{}).runCase(
		context.Background(),
		suite,
		suite.Spec.Cases[0],
		suite.Spec.Models[0],
		RunOptions{TargetWorkspace: workspace, OutputDir: t.TempDir(), Timeout: 20 * time.Millisecond, BootstrapTimeout: 20 * time.Millisecond},
		workspace,
		t.TempDir(),
	)

	if report.Success {
		t.Fatal("expected bootstrap timeout to fail case")
	}
	if report.FailureKind != "infra" {
		t.Fatalf("expected infra failure, got %q", report.FailureKind)
	}
	if !strings.Contains(report.Error, context.DeadlineExceeded.Error()) {
		t.Fatalf("expected deadline exceeded error, got %q", report.Error)
	}
	if report.DurationMS < 70 {
		t.Fatalf("expected case duration to reflect override budget, got %dms", report.DurationMS)
	}
}
