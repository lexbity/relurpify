package agenttest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appruntime "github.com/lexcodex/relurpify/app/relurpish/runtime"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
)

func newLoadedOllamaServer(t *testing.T, modelName string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	manifest := config.New(workspace).ManifestFile()
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

func TestResolveCaseTimeout(t *testing.T) {
	got, err := resolveCaseTimeout(RunOptions{Timeout: 30 * time.Second}, CaseSpec{Timeout: "2m"})
	if err != nil {
		t.Fatalf("resolveCaseTimeout case override: %v", err)
	}
	if got != 2*time.Minute {
		t.Fatalf("expected 2m timeout, got %s", got)
	}

	got, err = resolveCaseTimeout(RunOptions{Timeout: 30 * time.Second}, CaseSpec{})
	if err != nil {
		t.Fatalf("resolveCaseTimeout global: %v", err)
	}
	if got != 30*time.Second {
		t.Fatalf("expected 30s timeout, got %s", got)
	}

	if _, err := resolveCaseTimeout(RunOptions{}, CaseSpec{Timeout: "nope"}); err == nil {
		t.Fatal("expected invalid case timeout to fail")
	}
}

func TestResolveBootstrapTimeout(t *testing.T) {
	if got := resolveBootstrapTimeout(RunOptions{BootstrapTimeout: 5 * time.Second}); got != 5*time.Second {
		t.Fatalf("expected 5s bootstrap timeout, got %s", got)
	}
	if got := resolveBootstrapTimeout(RunOptions{}); got != 30*time.Second {
		t.Fatalf("expected default bootstrap timeout, got %s", got)
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
		TaskType: string(core.TaskTypeAnalysis),
	}) {
		t.Fatal("expected analysis case to restrict to explicit allowed capabilities")
	}
	if !shouldRestrictAllowedCapabilitiesForCase(CaseSpec{
		TaskType: string(core.TaskTypeCodeModification),
		Context:  map[string]any{"mode": "debug"},
	}) {
		t.Fatal("expected debug case to restrict to explicit allowed capabilities")
	}
	if shouldRestrictAllowedCapabilitiesForCase(CaseSpec{
		TaskType: string(core.TaskTypeCodeModification),
		Context:  map[string]any{"mode": "docs"},
	}) {
		t.Fatal("expected docs edit case to keep default capabilities merged")
	}
}

func TestSeedWorkflowRetrievalStateForCase(t *testing.T) {
	state := core.NewContext()
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
	raw, ok := state.Get("planner.workflow_retrieval")
	if !ok {
		t.Fatal("expected planner.workflow_retrieval state")
	}
	if got := fmt.Sprint(raw); !strings.Contains(got, "retrieval-backed") {
		t.Fatalf("expected planner workflow retrieval state, got %q", got)
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	if err := (&Runner{}).preflightSuite(context.Background(), suite, RunOptions{}, workspace, suite.Spec.Models); err != nil {
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	err := (&Runner{}).preflightSuite(context.Background(), suite, RunOptions{}, workspace, suite.Spec.Models)
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

func TestLookupOllamaModelProvenance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	provenance, err := lookupOllamaModelProvenance(server.URL, "qwen2.5-coder:14b")
	if err != nil {
		t.Fatalf("lookupOllamaModelProvenance: %v", err)
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

func TestApplyCaseControlFlowOverrideReturnsErrorForNonEmptyFlow(t *testing.T) {
	err := applyCaseControlFlowOverride(nil, CaseSpec{
		Overrides: CaseOverrideSpec{
			ControlFlow: "pipeline",
		},
	})
	if err == nil {
		t.Fatal("expected error for control_flow override, got nil")
	}
}

func TestApplyCaseControlFlowOverrideNoopForEmptyFlow(t *testing.T) {
	err := applyCaseControlFlowOverride(nil, CaseSpec{})
	if err != nil {
		t.Fatalf("expected no error for empty control_flow, got %v", err)
	}
}

func TestIncludeExpectedChangedFilesRestoresIgnoredExpectation(t *testing.T) {
	workflowStateRel := filepath.ToSlash(filepath.Join(config.DirName, "sessions", "workflow_state.db"))
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
	bootstrapAgentRuntime = func(_ string, opts appruntime.AgentBootstrapOptions) (*appruntime.BootstrappedAgentRuntime, error) {
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
