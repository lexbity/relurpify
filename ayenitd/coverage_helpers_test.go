package ayenitd

import (
	"context"
	"errors"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/framework/patterns"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/lexcodex/relurpify/platform/llm"
	"github.com/stretchr/testify/require"
)

type countCloser struct {
	closeCount atomic.Int32
	err        error
}

func (c *countCloser) Close() error {
	c.closeCount.Add(1)
	return c.err
}

type noopTelemetry struct{}

func (noopTelemetry) Emit(core.Event) {}

type fakeLanguageModel struct{}

func (fakeLanguageModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{}, nil
}

func (fakeLanguageModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (fakeLanguageModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{}, nil
}

func (fakeLanguageModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{}, nil
}

type fakeManagedBackend struct {
	closeCount atomic.Int32
	closeErr   error
}

func (f *fakeManagedBackend) Model() core.LanguageModel { return fakeLanguageModel{} }
func (f *fakeManagedBackend) Embedder() llm.Embedder    { return nil }
func (f *fakeManagedBackend) Capabilities() core.BackendCapabilities {
	return core.BackendCapabilities{}
}
func (f *fakeManagedBackend) Health(context.Context) (*llm.HealthReport, error) {
	return &llm.HealthReport{}, nil
}
func (f *fakeManagedBackend) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{Name: "model-a"}}, nil
}
func (f *fakeManagedBackend) Warm(context.Context) error { return nil }
func (f *fakeManagedBackend) Close() error {
	f.closeCount.Add(1)
	return f.closeErr
}
func (f *fakeManagedBackend) SetDebugLogging(bool) {}

type fakeCommandRunner struct{}

func (fakeCommandRunner) Run(context.Context, fsandbox.CommandRequest) (string, string, error) {
	return "", "", nil
}

func TestWorkspaceConfigAccessorsAndAgentLabel(t *testing.T) {
	cfg := WorkspaceConfig{
		InferenceProvider:          "provider",
		InferenceEndpoint:          "endpoint",
		InferenceModel:             "model",
		InferenceAPIKey:            "key",
		InferenceNativeToolCalling: true,
	}
	require.Equal(t, "provider", cfg.InferenceProviderValue())
	require.Equal(t, "endpoint", cfg.InferenceEndpointValue())
	require.Equal(t, "model", cfg.InferenceModelValue())
	require.Equal(t, "key", cfg.InferenceAPIKeyValue())
	require.True(t, cfg.InferenceNativeToolCallingValue())
	require.Equal(t, "default", cfg.AgentLabel())
	cfg.AgentName = "coding"
	require.Equal(t, "coding", cfg.AgentLabel())
}

func TestWorkspaceEnvironmentWithServiceRegistersService(t *testing.T) {
	sm := NewServiceManager()
	svc := &mockService{}
	env := WorkspaceEnvironment{ServiceManager: sm}
	updated := env.WithService("svc", svc)
	require.Same(t, env.ServiceManager, updated.ServiceManager)
	require.True(t, sm.Has("svc"))
	require.Same(t, svc, sm.Get("svc"))
}

func TestWorkspaceEnvironmentWithServiceNoManager(t *testing.T) {
	env := WorkspaceEnvironment{}
	updated := env.WithService("svc", &mockService{})
	require.Equal(t, env, updated)
}

func TestBrowserHelpersAndRegistrationGate(t *testing.T) {
	require.Nil(t, browserWorkspaceAgentSpec(nil))
	require.Nil(t, browserWorkspaceAgentSpec(&fauthorization.AgentRegistration{}))
	reg := &fauthorization.AgentRegistration{
		Manifest: &manifest.AgentManifest{
			Spec: manifest.ManifestSpec{
				Agent: &core.AgentRuntimeSpec{
					Browser: &core.AgentBrowserSpec{
						Enabled:         true,
						DefaultBackend:   "  firefox  ",
						AllowedBackends: []string{"cdp", "firefox"},
					},
				},
			},
		},
	}
	spec := browserWorkspaceAgentSpec(reg)
	require.NotNil(t, spec)
	require.True(t, shouldEnableBrowserWorkspaceService(spec))
	require.False(t, shouldEnableBrowserWorkspaceService(nil))
	require.Equal(t, "firefox", browserDefaultBackend(spec))
	require.Equal(t, []string{"cdp", "firefox"}, browserAllowedBackends(spec))
	allowed := browserAllowedBackends(spec)
	allowed[0] = "mutated"
	require.Equal(t, []string{"cdp", "firefox"}, spec.Browser.AllowedBackends)
	require.Equal(t, "cdp", browserDefaultBackend(nil))
	require.Nil(t, browserAllowedBackends(nil))

	err := registerBrowserWorkspaceService(context.Background(), WorkspaceConfig{Workspace: t.TempDir()}, reg, nil, nil, noopTelemetry{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "browser registry unavailable")
	require.NoError(t, registerBrowserWorkspaceService(context.Background(), WorkspaceConfig{Workspace: t.TempDir()}, &fauthorization.AgentRegistration{}, nil, nil, noopTelemetry{}))
}

func TestRegisterBrowserWorkspaceServiceSuccess(t *testing.T) {
	workspace := t.TempDir()
	registry := capability.NewRegistry()
	sm := NewServiceManager()
	reg := &fauthorization.AgentRegistration{
		ID: "agent-browser",
		Manifest: &manifest.AgentManifest{
			Spec: manifest.ManifestSpec{
				Agent: &core.AgentRuntimeSpec{
					Mode:  core.AgentModePrimary,
					Model: core.AgentModelConfig{Provider: "ollama", Name: "stub"},
					Browser: &core.AgentBrowserSpec{
						Enabled: true,
					},
				},
			},
		},
	}
	require.NoError(t, registerBrowserWorkspaceService(context.Background(), WorkspaceConfig{Workspace: workspace}, reg, registry, sm, noopTelemetry{}))
	require.NotNil(t, sm.Get("browser"))
	require.True(t, registry.HasCapability("browser"))
}

func TestServiceManagerListIDs(t *testing.T) {
	sm := NewServiceManager()
	sm.Register("b", &mockService{})
	sm.Register("a", &mockService{})
	ids := sm.ListIDs()
	sort.Strings(ids)
	require.Equal(t, []string{"a", "b"}, ids)
}

func TestWorkspaceLifecycleAndClosers(t *testing.T) {
	workflowStore, _, _, _, _, patternDB, err := openRuntimeStores(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = workflowStore.Close()
		_ = patternDB.Close()
	})
	backend := &fakeManagedBackend{}
	svc := &mockService{}
	sm := NewServiceManager()
	sm.Register("svc", svc)
	ws := &Workspace{
		Environment: WorkspaceEnvironment{
			Scheduler:     NewServiceScheduler(),
			WorkflowStore:  workflowStore,
			ServiceManager: sm,
		},
		ServiceManager: sm,
		Backend:       backend,
	}

	logCloser := &countCloser{}
	patternCloser := &countCloser{}
	eventCloser := &countCloser{}
	ws.logFile = logCloser
	ws.patternDB = patternCloser
	ws.eventLog = eventCloser

	logOut, patternOut, eventOut := ws.StealClosers()
	require.Same(t, logCloser, logOut)
	require.Same(t, patternCloser, patternOut)
	require.Same(t, eventCloser, eventOut)
	require.Nil(t, ws.logFile)
	require.Nil(t, ws.patternDB)
	require.Nil(t, ws.eventLog)

	require.Same(t, sm.Get("svc"), ws.GetService("svc"))
	require.Len(t, ws.ListServices(), 1)
	require.NoError(t, ws.Close())
	require.EqualValues(t, 1, backend.closeCount.Load())
	require.EqualValues(t, 0, logCloser.closeCount.Load())
	require.EqualValues(t, 0, patternCloser.closeCount.Load())
	require.EqualValues(t, 0, eventCloser.closeCount.Load())
	require.False(t, sm.Has("svc"))
}

func TestWorkspaceCloseContinuesAfterServiceError(t *testing.T) {
	workflowStore, _, _, _, _, patternDB, err := openRuntimeStores(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = workflowStore.Close()
		_ = patternDB.Close()
	})
	logCloser := &countCloser{}
	patternCloser := &countCloser{}
	eventCloser := &countCloser{}
	backend := &fakeManagedBackend{}
	sm := NewServiceManager()
	sm.Register("svc", &mockService{stopErr: context.Canceled})
	ws := &Workspace{
		Environment: WorkspaceEnvironment{
			Scheduler:     NewServiceScheduler(),
			WorkflowStore:  workflowStore,
			ServiceManager: sm,
		},
		Backend:       backend,
		logFile:       logCloser,
		patternDB:     patternCloser,
		eventLog:      eventCloser,
		ServiceManager: sm,
	}
	err = ws.Close()
	require.Error(t, err)
	require.Contains(t, err.Error(), "stop services")
	require.EqualValues(t, 1, backend.closeCount.Load())
	require.EqualValues(t, 1, logCloser.closeCount.Load())
	require.EqualValues(t, 1, patternCloser.closeCount.Load())
	require.EqualValues(t, 1, eventCloser.closeCount.Load())
}

func TestWorkspaceCloseAggregatesOwnedCloserErrors(t *testing.T) {
	workflowStore, _, _, _, _, patternDB, err := openRuntimeStores(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = workflowStore.Close()
		_ = patternDB.Close()
	})
	logCloser := &countCloser{err: context.Canceled}
	patternCloser := &countCloser{err: context.Canceled}
	eventCloser := &countCloser{err: context.Canceled}
	backend := &fakeManagedBackend{closeErr: context.Canceled}
	sm := NewServiceManager()
	sm.Register("svc", &mockService{})
	ws := &Workspace{
		Environment: WorkspaceEnvironment{
			Scheduler:     NewServiceScheduler(),
			WorkflowStore:  workflowStore,
			ServiceManager: sm,
		},
		Backend:       backend,
		logFile:       logCloser,
		patternDB:     patternCloser,
		eventLog:      eventCloser,
		ServiceManager: sm,
	}
	err = ws.Close()
	require.Error(t, err)
	require.EqualValues(t, 1, backend.closeCount.Load())
	require.EqualValues(t, 1, logCloser.closeCount.Load())
	require.EqualValues(t, 1, patternCloser.closeCount.Load())
	require.EqualValues(t, 1, eventCloser.closeCount.Load())
}

func TestWorkspaceCloseClosesOwnedClosers(t *testing.T) {
	workflowStore, _, _, _, _, patternDB, err := openRuntimeStores(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = workflowStore.Close()
		_ = patternDB.Close()
	})
	logCloser := &countCloser{}
	patternCloser := &countCloser{}
	eventCloser := &countCloser{}
	backend := &fakeManagedBackend{}
	sm := NewServiceManager()
	sm.Register("svc", &mockService{})
	ws := &Workspace{
		Environment: WorkspaceEnvironment{
			Scheduler:     NewServiceScheduler(),
			WorkflowStore:  workflowStore,
			ServiceManager: sm,
		},
		Backend:       backend,
		logFile:       logCloser,
		patternDB:     patternCloser,
		eventLog:      eventCloser,
		ServiceManager: sm,
	}
	require.NoError(t, ws.Close())
	require.EqualValues(t, 1, backend.closeCount.Load())
	require.EqualValues(t, 1, logCloser.closeCount.Load())
	require.EqualValues(t, 1, patternCloser.closeCount.Load())
	require.EqualValues(t, 1, eventCloser.closeCount.Load())
}

func TestWorkspaceRestartStopsAndStartsServices(t *testing.T) {
	sm := NewServiceManager()
	svc := &mockService{}
	sm.Register("svc", svc)
	ws := &Workspace{
		Environment: WorkspaceEnvironment{
			Scheduler:     NewServiceScheduler(),
			ServiceManager: sm,
		},
		ServiceManager: sm,
	}
	require.NoError(t, ws.Restart(context.Background()))
	require.EqualValues(t, 1, svc.stopCount.Load())
	require.EqualValues(t, 1, svc.startCount.Load())
}

func TestWorkspaceStopServicesStopsManager(t *testing.T) {
	sm := NewServiceManager()
	svc := &mockService{}
	sm.Register("svc", svc)
	ws := &Workspace{
		Environment: WorkspaceEnvironment{
			Scheduler:     NewServiceScheduler(),
			ServiceManager: sm,
		},
		ServiceManager: sm,
	}
	require.NoError(t, ws.stopServices())
	require.EqualValues(t, 1, svc.stopCount.Load())
}

func TestWorkspaceGetServiceNilAndRestartError(t *testing.T) {
	require.Nil(t, (&Workspace{}).GetService("missing"))
	require.Nil(t, (&Workspace{}).ListServices())

	sm := NewServiceManager()
	sm.Register("svc", &mockService{stopErr: context.Canceled})
	ws := &Workspace{
		Environment: WorkspaceEnvironment{
			Scheduler:     NewServiceScheduler(),
			ServiceManager: sm,
		},
		ServiceManager: sm,
	}
	err := ws.Restart(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "stop services for restart")
}

func TestWorkspaceRestartNilManagerReturnsError(t *testing.T) {
	err := (&Workspace{}).Restart(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "service manager unavailable")
}

func TestOpenRuntimeStoresHappyPathAndFailures(t *testing.T) {
	workspace := t.TempDir()
	workflowStore, planStore, patternStore, commentStore, knowledgeStore, patternDB, err := openRuntimeStores(workspace)
	require.NoError(t, err)
	require.NotNil(t, workflowStore)
	require.NotNil(t, planStore)
	require.NotNil(t, patternStore)
	require.NotNil(t, commentStore)
	require.NotNil(t, knowledgeStore)
	require.NotNil(t, patternDB)
	require.NoError(t, workflowStore.Close())
	require.NoError(t, patternDB.Close())

	blockedSessions := filepath.Join(t.TempDir(), config.DirName, "sessions")
	require.NoDirExists(t, blockedSessions)
	require.NoError(t, os.MkdirAll(filepath.Dir(blockedSessions), 0o755))
	require.NoError(t, os.WriteFile(blockedSessions, []byte("block"), 0o644))
	_, _, _, _, _, _, err = openRuntimeStores(filepath.Dir(filepath.Dir(blockedSessions)))
	require.Error(t, err)
	require.Contains(t, err.Error(), "create sessions directory")

	blockedMemoryRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(blockedMemoryRoot, config.DirName, "sessions"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(blockedMemoryRoot, config.DirName, "memory"), []byte("block"), 0o644))
	_, _, _, _, _, _, err = openRuntimeStores(blockedMemoryRoot)
	require.Error(t, err)
	require.Contains(t, err.Error(), "create memory directory")
}

func TestOpenRuntimeStoresFailureBranchesWithSeams(t *testing.T) {
	workspace := t.TempDir()

	origMkdirAllFn := mkdirAllFn
	origNewSQLiteWorkflowStateStoreFn := newSQLiteWorkflowStateStoreFn
	origNewSQLitePlanStoreFn := newSQLitePlanStoreFn
	origOpenPatternsSQLiteFn := openPatternsSQLiteFn
	origNewSQLitePatternStoreFn := newSQLitePatternStoreFn
	origNewSQLiteCommentStoreFn := newSQLiteCommentStoreFn
	origNewKnowledgeStoreFn := newKnowledgeStoreFn
	t.Cleanup(func() {
		mkdirAllFn = origMkdirAllFn
		newSQLiteWorkflowStateStoreFn = origNewSQLiteWorkflowStateStoreFn
		newSQLitePlanStoreFn = origNewSQLitePlanStoreFn
		openPatternsSQLiteFn = origOpenPatternsSQLiteFn
		newSQLitePatternStoreFn = origNewSQLitePatternStoreFn
		newSQLiteCommentStoreFn = origNewSQLiteCommentStoreFn
		newKnowledgeStoreFn = origNewKnowledgeStoreFn
	})

	mkdirAllFn = func(string, os.FileMode) error { return errors.New("mkdir boom") }
	_, _, _, _, _, _, err := openRuntimeStores(workspace)
	require.Error(t, err)
	require.Contains(t, err.Error(), "create sessions directory")

	mkdirAllFn = origMkdirAllFn
	newSQLiteWorkflowStateStoreFn = func(string) (*memorydb.SQLiteWorkflowStateStore, error) {
		return nil, errors.New("workflow boom")
	}
	_, _, _, _, _, _, err = openRuntimeStores(workspace)
	require.Error(t, err)
	require.Contains(t, err.Error(), "open workflow state store")

	newSQLiteWorkflowStateStoreFn = origNewSQLiteWorkflowStateStoreFn
	newSQLitePlanStoreFn = func(*sql.DB) (*frameworkplan.SQLitePlanStore, error) {
		return nil, errors.New("plan boom")
	}
	_, _, _, _, _, _, err = openRuntimeStores(workspace)
	require.Error(t, err)
	require.Contains(t, err.Error(), "open living plan store")

	newSQLitePlanStoreFn = origNewSQLitePlanStoreFn
	openPatternsSQLiteFn = func(string) (*sql.DB, error) {
		return nil, errors.New("patterns boom")
	}
	_, _, _, _, _, _, err = openRuntimeStores(workspace)
	require.Error(t, err)
	require.Contains(t, err.Error(), "open patterns store")

	openPatternsSQLiteFn = origOpenPatternsSQLiteFn
	newSQLitePatternStoreFn = func(*sql.DB) (*patterns.SQLitePatternStore, error) {
		return nil, errors.New("pattern store boom")
	}
	_, _, _, _, _, _, err = openRuntimeStores(workspace)
	require.Error(t, err)
	require.Contains(t, err.Error(), "open pattern catalog")

	newSQLitePatternStoreFn = origNewSQLitePatternStoreFn
	newSQLiteCommentStoreFn = func(*sql.DB) (*patterns.SQLiteCommentStore, error) {
		return nil, errors.New("comment store boom")
	}
	_, _, _, _, _, _, err = openRuntimeStores(workspace)
	require.Error(t, err)
	require.Contains(t, err.Error(), "open comment catalog")
}

func TestProbeWorkspaceBackendEdgeCases(t *testing.T) {
	workspace := t.TempDir()
	sessionsDir := filepath.Join(workspace, config.DirName, "sessions")
	require.NoError(t, os.MkdirAll(filepath.Dir(sessionsDir), 0o755))
	require.NoError(t, os.WriteFile(sessionsDir, []byte("block"), 0o644))
	ok, msg := checkSQLiteWritable(workspace)
	require.False(t, ok)
	require.Contains(t, msg, "cannot create sessions dir")

	_, msg = checkDiskSpace("/definitely/not/a/real/path", 1<<40)
	require.True(t, strings.Contains(msg, "cannot check disk space") || strings.Contains(msg, "sufficient"))

	results := ProbeWorkspace(WorkspaceConfig{Workspace: workspace, InferenceEndpoint: "endpoint", InferenceProvider: "stub"}, &fakeManagedBackend{
	})
	r := findProbeResult(t, results, "workspace_directory")
	require.True(t, r.OK)
	r = findProbeResult(t, results, "sqlite_writable")
	require.False(t, r.OK)
	r = findProbeResult(t, results, "disk_space")
	require.True(t, r.Required == false)
}

func TestCheckInferenceBackendEdgeCases(t *testing.T) {
	cfg := WorkspaceConfig{InferenceEndpoint: "endpoint", InferenceProvider: "stub", InferenceModel: "missing"}
	ok, msg := checkInferenceBackend(cfg, &backendWithModels{})
	require.False(t, ok)
	require.Contains(t, msg, "not found")

	ok, msg = checkInferenceBackend(cfg, &backendWithNoModels{})
	require.False(t, ok)
	require.Contains(t, msg, "returned no models")

	ok, msg = checkInferenceBackend(cfg, &backendWithListError{})
	require.False(t, ok)
	require.Contains(t, msg, "model list failed")
}

type backendWithNoModels struct{}

type backendWithModels struct{}

func (backendWithModels) Model() core.LanguageModel { return fakeLanguageModel{} }
func (backendWithModels) Embedder() llm.Embedder    { return nil }
func (backendWithModels) Capabilities() core.BackendCapabilities {
	return core.BackendCapabilities{}
}
func (backendWithModels) Health(context.Context) (*llm.HealthReport, error) { return &llm.HealthReport{}, nil }
func (backendWithModels) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{Name: "model-a"}}, nil
}
func (backendWithModels) Warm(context.Context) error { return nil }
func (backendWithModels) Close() error               { return nil }
func (backendWithModels) SetDebugLogging(bool)       {}

func (backendWithNoModels) Model() core.LanguageModel { return fakeLanguageModel{} }
func (backendWithNoModels) Embedder() llm.Embedder    { return nil }
func (backendWithNoModels) Capabilities() core.BackendCapabilities {
	return core.BackendCapabilities{}
}
func (backendWithNoModels) Health(context.Context) (*llm.HealthReport, error) { return &llm.HealthReport{}, nil }
func (backendWithNoModels) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return nil, nil
}
func (backendWithNoModels) Warm(context.Context) error { return nil }
func (backendWithNoModels) Close() error               { return nil }
func (backendWithNoModels) SetDebugLogging(bool)       {}

type backendWithListError struct{}

func (backendWithListError) Model() core.LanguageModel { return fakeLanguageModel{} }
func (backendWithListError) Embedder() llm.Embedder    { return nil }
func (backendWithListError) Capabilities() core.BackendCapabilities {
	return core.BackendCapabilities{}
}
func (backendWithListError) Health(context.Context) (*llm.HealthReport, error) { return &llm.HealthReport{}, nil }
func (backendWithListError) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return nil, errors.New("boom")
}
func (backendWithListError) Warm(context.Context) error { return nil }
func (backendWithListError) Close() error               { return nil }
func (backendWithListError) SetDebugLogging(bool)       {}

func TestConfigHelpersAndTelemetry(t *testing.T) {
	cfg := WorkspaceConfig{
		Workspace:         t.TempDir(),
		ManifestPath:      filepath.Join(t.TempDir(), "manifest.yaml"),
		InferenceEndpoint: "http://localhost:11434",
		InferenceProvider: "ollama",
	}
	require.NoError(t, os.WriteFile(cfg.ManifestPath, []byte("kind: AgentManifest\n"), 0o644))
	require.NoError(t, validateConfig(cfg))

	out, logger, tel, err := setupTelemetry(cfg)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotNil(t, logger)
	require.NotNil(t, tel)
	require.NoError(t, out.Close())

	require.Error(t, validateConfig(WorkspaceConfig{}))

	embedderCfg := embedderCfgFromConfig(WorkspaceConfig{}, "fallback-model")
	require.Equal(t, "ollama", embedderCfg.Provider)
	require.Equal(t, "fallback-model", embedderCfg.Model)
}

func TestSetupTelemetryWarnsOnTelemetryFileInitError(t *testing.T) {
	dir := t.TempDir()
	telemetryPath := filepath.Join(dir, "telemetry")
	require.NoError(t, os.MkdirAll(telemetryPath, 0o755))
	cfg := WorkspaceConfig{
		Workspace:         dir,
		ManifestPath:      filepath.Join(dir, "manifest.yaml"),
		InferenceEndpoint: "http://localhost:11434",
		TelemetryPath:     telemetryPath,
	}
	require.NoError(t, os.WriteFile(cfg.ManifestPath, []byte("kind: AgentManifest\n"), 0o644))

	out, logger, tel, err := setupTelemetry(cfg)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotNil(t, logger)
	require.NotNil(t, tel)
	require.NoError(t, out.Close())
}

func TestSetupTelemetryUsesTelemetryPath(t *testing.T) {
	dir := t.TempDir()
	telemetryPath := filepath.Join(dir, "telemetry", "events.jsonl")
	cfg := WorkspaceConfig{
		Workspace:         dir,
		ManifestPath:      filepath.Join(dir, "manifest.yaml"),
		InferenceEndpoint: "http://localhost:11434",
		TelemetryPath:     telemetryPath,
	}
	require.NoError(t, os.WriteFile(cfg.ManifestPath, []byte("kind: AgentManifest\n"), 0o644))

	out, logger, tel, err := setupTelemetry(cfg)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotNil(t, logger)
	require.NotNil(t, tel)
	require.NoError(t, out.Close())
	require.FileExists(t, telemetryPath)
}

func findProbeResult(t *testing.T, results []ProbeResult, name string) ProbeResult {
	t.Helper()
	for _, r := range results {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("probe result %q not found", name)
	return ProbeResult{}
}
