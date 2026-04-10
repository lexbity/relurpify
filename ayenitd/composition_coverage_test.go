package ayenitd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"log"
	"testing"

	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/retrieval"
	"github.com/lexcodex/relurpify/framework/search"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
	frameworkskills "github.com/lexcodex/relurpify/framework/skills"
	"github.com/lexcodex/relurpify/platform/llm"
	"github.com/stretchr/testify/require"
)

const openManifestYAML = `apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: demo
spec:
  image: ghcr.io/example/runtime:latest
  runtime: gvisor
  defaults:
    permissions:
      executables:
        - binary: bash
          args: ["-c", "*"]
  permissions:
    filesystem:
      - path: /tmp
        action: fs:read
  agent:
    implementation: react
    mode: primary
    model:
      provider: ollama
      name: stub-model
`

type openTestState struct {
	backend *fakeManagedBackend
}

type permissionTelemetry struct{}

func (permissionTelemetry) Emit(core.Event) {}
func (permissionTelemetry) EmitPermissionEvent(context.Context, core.PermissionDescriptor, string, string, map[string]interface{}) {
}

func mustOpenTestPermissionManager(t *testing.T, workspace string) *authorization.PermissionManager {
	t.Helper()
	pm, err := authorization.NewPermissionManager(workspace, &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: workspace + "/**"}},
	}, nil, nil)
	require.NoError(t, err)
	pm.SetDefaultPolicy(core.AgentPermissionAllow)
	return pm
}

func withOpenSeams(t *testing.T, state *openTestState) {
	t.Helper()
	origNewLLMBackendFn := newLLMBackendFn
	origApplyProfileFn := applyProfileFn
	origProbeWorkspaceFn := probeWorkspaceFn
	origLoadAgentManifestSnapshotFn := loadAgentManifestSnapshotFn
	origRegisterAgentFn := registerAgentFn
	origNewCommandRunnerFn := newCommandRunnerFn
	origNewHybridMemoryFn := newHybridMemoryFn
	origNewProfileRegistryFn := newProfileRegistryFn
	origNewInstrumentedModelFn := newInstrumentedModelFn
	origBootstrapAgentRuntimeFn := bootstrapAgentRuntimeFn
	origNewEmbedderFn := newEmbedderFn
	origRegisterBrowserWorkspaceServiceFn := registerBrowserWorkspaceServiceFn
	origNewServiceSchedulerFn := newServiceSchedulerFn
	origSetupTelemetryFn := setupTelemetryFn
	origOpenRuntimeStoresFn := openRuntimeStoresFn

	newLLMBackendFn = func(llm.ProviderConfig) (llm.ManagedBackend, error) {
		return state.backend, nil
	}
	applyProfileFn = func(target any, profile *llm.ModelProfile) bool {
		return true
	}
	probeWorkspaceFn = func(cfg WorkspaceConfig, backend llm.ManagedBackend) []ProbeResult {
		return []ProbeResult{
			{Name: "workspace_directory", Required: true, OK: true},
			{Name: "sqlite_writable", Required: true, OK: true},
			{Name: "inference_backend", Required: true, OK: true},
			{Name: "disk_space", Required: false, OK: true},
		}
	}
	loadAgentManifestSnapshotFn = manifest.LoadAgentManifestSnapshot
	registerAgentFn = func(ctx context.Context, cfg authorization.RuntimeConfig) (*authorization.AgentRegistration, error) {
		return &authorization.AgentRegistration{
			ID:       "agent-1",
			Manifest: cfg.ManifestSnapshot.Manifest,
			Permissions: mustOpenTestPermissionManager(t, cfg.BaseFS),
		}, nil
	}
	newCommandRunnerFn = func(manifest *manifest.AgentManifest, runtime fsandbox.SandboxRuntime, workspace string) (fsandbox.CommandRunner, error) {
		return fakeCommandRunner{}, nil
	}
	newHybridMemoryFn = memory.NewHybridMemory
	newProfileRegistryFn = llm.NewProfileRegistry
	newInstrumentedModelFn = func(inner core.LanguageModel, telemetry core.Telemetry, debug bool) core.LanguageModel {
		return fakeLanguageModel{}
	}
	bootstrapAgentRuntimeFn = func(workspace string, opts AgentBootstrapOptions) (*BootstrappedAgentRuntime, error) {
		registry := capability.NewRegistry()
		env := WorkspaceEnvironment{
			Config:         &core.Config{Name: "demo", Model: "stub"},
			Registry:       registry,
			Memory:         opts.Memory,
			WorkflowStore:  opts.WorkflowStore,
			ServiceManager: nil,
		}
		return &BootstrappedAgentRuntime{
			Registry:    registry,
			Environment: env,
			AgentSpec:   opts.Manifest.Spec.Agent,
			AgentConfig: &core.Config{Name: opts.ConfigName, Model: opts.InferenceModel, AgentSpec: opts.Manifest.Spec.Agent},
			Backend:     opts.Backend,
			Memory:      opts.Memory,
		}, nil
	}
	newEmbedderFn = func(backend llm.ManagedBackend, cfg retrieval.EmbedderConfig) (retrieval.Embedder, error) {
		return nil, nil
	}
	registerBrowserWorkspaceServiceFn = func(context.Context, WorkspaceConfig, *authorization.AgentRegistration, *capability.Registry, *ServiceManager, core.Telemetry) error {
		return nil
	}
	newServiceSchedulerFn = NewServiceScheduler
	setupTelemetryFn = setupTelemetry
	openRuntimeStoresFn = openRuntimeStores

	t.Cleanup(func() {
		newLLMBackendFn = origNewLLMBackendFn
		applyProfileFn = origApplyProfileFn
		probeWorkspaceFn = origProbeWorkspaceFn
		loadAgentManifestSnapshotFn = origLoadAgentManifestSnapshotFn
		registerAgentFn = origRegisterAgentFn
		newCommandRunnerFn = origNewCommandRunnerFn
		newHybridMemoryFn = origNewHybridMemoryFn
		newProfileRegistryFn = origNewProfileRegistryFn
		newInstrumentedModelFn = origNewInstrumentedModelFn
		bootstrapAgentRuntimeFn = origBootstrapAgentRuntimeFn
		newEmbedderFn = origNewEmbedderFn
		registerBrowserWorkspaceServiceFn = origRegisterBrowserWorkspaceServiceFn
		newServiceSchedulerFn = origNewServiceSchedulerFn
		setupTelemetryFn = origSetupTelemetryFn
		openRuntimeStoresFn = origOpenRuntimeStoresFn
	})
}

func TestOpenSuccessWithSeams(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := writeOpenManifest(t, workspace)

	state := &openTestState{backend: &fakeManagedBackend{}}
	withOpenSeams(t, state)

	ws, err := Open(context.Background(), WorkspaceConfig{
		Workspace:         workspace,
		ManifestPath:      manifestPath,
		InferenceEndpoint: "http://localhost:11434",
		MemoryPath:        filepath.Join(workspace, "memory"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })
	require.NotNil(t, ws.Environment.Registry)
	require.NotNil(t, ws.Environment.Scheduler)
	require.NotNil(t, ws.Logger)
	require.NotNil(t, ws.Telemetry)
	require.NotNil(t, ws.ServiceManager)
	require.NotNil(t, ws.GetService("scheduler"))
	require.NotNil(t, ws.GetService("bkc.git_watcher"))
}

func TestOpenWithTelemetryAndEvents(t *testing.T) {
	workspace := t.TempDir()
	manifestPath := writeOpenManifest(t, workspace)
	state := &openTestState{backend: &fakeManagedBackend{}}
	withOpenSeams(t, state)
	setupTelemetryFn = func(cfg WorkspaceConfig) (*os.File, *log.Logger, core.Telemetry, error) {
		logFile, logger, _, err := setupTelemetry(cfg)
		if err != nil {
			return nil, nil, nil, err
		}
		return logFile, logger, permissionTelemetry{}, nil
	}

	ws, err := Open(context.Background(), WorkspaceConfig{
		Workspace:         workspace,
		ManifestPath:      manifestPath,
		InferenceEndpoint: "http://localhost:11434",
		MemoryPath:        filepath.Join(workspace, "memory"),
		LogPath:           filepath.Join(workspace, "logs", "ayenitd.log"),
		TelemetryPath:     filepath.Join(workspace, "telemetry", "events.jsonl"),
		EventsPath:        filepath.Join(workspace, "events.db"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })
	require.NotNil(t, ws.Telemetry)
	require.NotNil(t, ws.Registration.Permissions)
}

func TestOpenBackendFailure(t *testing.T) {
	state := &openTestState{backend: &fakeManagedBackend{}}
	withOpenSeams(t, state)
	newLLMBackendFn = func(llm.ProviderConfig) (llm.ManagedBackend, error) {
		return nil, errors.New("backend boom")
	}
	_, err := Open(context.Background(), WorkspaceConfig{
		Workspace:         t.TempDir(),
		ManifestPath:      filepath.Join(t.TempDir(), "agent.manifest.yaml"),
		InferenceEndpoint: "http://localhost:11434",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "build inference backend")
}

func TestOpenFailureBranches(t *testing.T) {
	state := &openTestState{backend: &fakeManagedBackend{}}
	workspace := t.TempDir()
	manifestPath := writeOpenManifest(t, workspace)
	baseCfg := WorkspaceConfig{
		Workspace:         workspace,
		ManifestPath:      manifestPath,
		InferenceEndpoint: "http://localhost:11434",
		MemoryPath:        filepath.Join(workspace, "memory"),
	}

	cases := []struct {
		name     string
		apply    func()
		wantText string
	}{
		{
			name: "telemetry",
			apply: func() {
				setupTelemetryFn = func(cfg WorkspaceConfig) (*os.File, *log.Logger, core.Telemetry, error) {
					return nil, nil, nil, errors.New("telemetry boom")
				}
			},
			wantText: "telemetry boom",
		},
		{
			name: "manifest",
			apply: func() {
				loadAgentManifestSnapshotFn = func(string) (*manifest.AgentManifestSnapshot, error) {
					return nil, errors.New("manifest boom")
				}
			},
			wantText: "load manifest snapshot",
		},
		{
			name: "register",
			apply: func() {
				registerAgentFn = func(context.Context, authorization.RuntimeConfig) (*authorization.AgentRegistration, error) {
					return nil, errors.New("register boom")
				}
			},
			wantText: "sandbox registration failed",
		},
		{
			name: "runner",
			apply: func() {
				newCommandRunnerFn = func(*manifest.AgentManifest, fsandbox.SandboxRuntime, string) (fsandbox.CommandRunner, error) {
					return nil, errors.New("runner boom")
				}
			},
			wantText: "runner boom",
		},
		{
			name: "memory",
			apply: func() {
				newHybridMemoryFn = func(string) (*memory.HybridMemory, error) {
					return nil, errors.New("memory boom")
				}
			},
			wantText: "memory init",
		},
		{
			name: "profiles",
			apply: func() {
				newProfileRegistryFn = func(string) (*llm.ProfileRegistry, error) {
					return nil, errors.New("profiles boom")
				}
			},
			wantText: "load model profiles",
		},
		{
			name: "bootstrap",
			apply: func() {
				bootstrapAgentRuntimeFn = func(string, AgentBootstrapOptions) (*BootstrappedAgentRuntime, error) {
					return nil, errors.New("bootstrap boom")
				}
			},
			wantText: "bootstrap boom",
		},
		{
			name: "embedder",
			apply: func() {
				newEmbedderFn = func(llm.ManagedBackend, retrieval.EmbedderConfig) (retrieval.Embedder, error) {
					return nil, errors.New("embedder boom")
				}
			},
			wantText: "build embedder",
		},
		{
			name: "browser",
			apply: func() {
				registerBrowserWorkspaceServiceFn = func(context.Context, WorkspaceConfig, *authorization.AgentRegistration, *capability.Registry, *ServiceManager, core.Telemetry) error {
					return errors.New("browser boom")
				}
			},
			wantText: "browser boom",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withOpenSeams(t, state)
			tc.apply()
			_, err := Open(context.Background(), baseCfg)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantText)
		})
	}
}

func TestBootstrapAgentRuntimeHelpers(t *testing.T) {
	dir := t.TempDir()
	defsDir := filepath.Join(dir, "defs")
	require.NoError(t, os.MkdirAll(defsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(defsDir, "alpha.yaml"), []byte(`apiVersion: relurpify/v1alpha1
kind: AgentDefinition
metadata:
  name: alpha
spec:
  implementation: react
  mode: primary
  model:
    provider: ollama
    name: model-a
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(defsDir, "ignore.txt"), []byte("ignore"), 0o644))
	defs, err := loadAgentDefinitions(defsDir)
	require.NoError(t, err)
	require.Contains(t, defs, "alpha")
	require.Equal(t, "alpha", defs["alpha"].Name)
	require.Len(t, selectedAgentDefinitionOverlays("alpha", defs), 1)
	require.Nil(t, selectedAgentDefinitionOverlays("missing", defs))
	require.Nil(t, selectedAgentDefinitionOverlays("alpha", nil))

	brokenDir := filepath.Join(dir, "broken")
	require.NoError(t, os.MkdirAll(brokenDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(brokenDir, "broken.yaml"), []byte("spec: ["), 0o644))
	_, err = loadAgentDefinitions(brokenDir)
	require.Error(t, err)

	candidates := toCapabilityPlanCandidates([]frameworkskills.SkillCapabilityCandidate{{
		Descriptor: core.CapabilityDescriptor{Name: "a"},
	}})
	require.Len(t, candidates, 1)
	require.Equal(t, "a", candidates[0].Descriptor.Name)

	boot, err := BootstrapAgentRuntime(dir, AgentBootstrapOptions{
		AgentID:    "agent-1",
		AgentName:  "alpha",
		ConfigName: "",
		Manifest: &manifest.AgentManifest{
			Metadata: manifest.ManifestMetadata{Name: "demo"},
			Spec: manifest.ManifestSpec{
				Agent: &core.AgentRuntimeSpec{
					Model: core.AgentModelConfig{Name: "model-a"},
				},
			},
		},
		AgentsDir:    defsDir,
		Runner:       fakeCommandRunner{},
		Memory:       mustHybridMemory(t, filepath.Join(dir, "memory")),
		Telemetry:    noopTelemetry{},
		SkipASTIndex: true,
	})
	require.NoError(t, err)
	require.NotNil(t, boot.Registry)
	require.NotNil(t, boot.Environment.Registry)
	require.Equal(t, "demo", boot.AgentConfig.Name)
	require.Equal(t, 8, boot.AgentConfig.MaxIterations)
	require.Contains(t, boot.AgentDefinitions, "alpha")

	_, err = BootstrapAgentRuntime("", AgentBootstrapOptions{})
	require.Error(t, err)
	_, err = BootstrapAgentRuntime(dir, AgentBootstrapOptions{Manifest: &manifest.AgentManifest{}, Runner: fakeCommandRunner{}})
	require.Error(t, err)
	_, err = BootstrapAgentRuntime(dir, AgentBootstrapOptions{Manifest: &manifest.AgentManifest{Spec: manifest.ManifestSpec{Agent: &core.AgentRuntimeSpec{}}}})
	require.Error(t, err)
}

func TestBootstrapAgentRuntimeWithOverridesAndPolicies(t *testing.T) {
	dir := t.TempDir()
	defsDir := filepath.Join(dir, "defs")
	require.NoError(t, os.MkdirAll(defsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(defsDir, "ops.yaml"), []byte(`apiVersion: relurpify/v1alpha1
kind: AgentDefinition
metadata:
  name: ops
spec:
  implementation: react
  mode: primary
  model:
    provider: ollama
    name: override-model
`), 0o644))

	pm := mustOpenTestPermissionManager(t, dir)
	boot, err := BootstrapAgentRuntime(dir, AgentBootstrapOptions{
		AgentID:    "agent-override",
		AgentName:  "ops",
		ConfigName: "custom-name",
		AgentsDir:  defsDir,
		Manifest: &manifest.AgentManifest{
			Metadata: manifest.ManifestMetadata{Name: "manifest-name"},
			Spec: manifest.ManifestSpec{
				Agent: &core.AgentRuntimeSpec{
					Model: core.AgentModelConfig{Name: "manifest-model"},
				},
			},
		},
		AgentSpec: &core.AgentRuntimeSpec{
			Implementation: "react",
			Mode:           core.AgentModePrimary,
			Model:          core.AgentModelConfig{Name: "override-model"},
		},
		Runner:           fakeCommandRunner{},
		Memory:           mustHybridMemory(t, filepath.Join(dir, "memory")),
		Telemetry:        noopTelemetry{},
		PermissionManager: pm,
		SkipASTIndex:     true,
	})
	require.NoError(t, err)
	require.NotNil(t, boot)
	require.Equal(t, "custom-name", boot.AgentConfig.Name)
	require.Equal(t, "override-model", boot.AgentConfig.Model)
	require.NotNil(t, boot.Environment.PermissionManager)
	require.Contains(t, boot.AgentDefinitions, "ops")
}

func mustHybridMemory(t *testing.T, path string) *memory.HybridMemory {
	t.Helper()
	mem, err := memory.NewHybridMemory(path)
	require.NoError(t, err)
	return mem
}

func writeOpenManifest(t *testing.T, workspace string) string {
	t.Helper()
	manifestPath := filepath.Join(workspace, config.DirName, "agent.manifest.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.WriteFile(manifestPath, []byte(openManifestYAML), 0o644))
	return manifestPath
}

func TestCapabilityBundleCoverage(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main(){}\n"), 0o644))
	bundle, err := BuildBuiltinCapabilityBundle(dir, fakeCommandRunner{}, CapabilityRegistryOptions{SkipASTIndex: true})
	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.NotNil(t, bundle.Registry)
	require.NotNil(t, bundle.IndexManager)
	require.NotNil(t, bundle.SearchEngine)
	require.True(t, bundle.Registry.HasCapability("go_test"))
	require.True(t, bundle.Registry.HasCapability("file_search"))

	_, err = BuildBuiltinCapabilityBundle(dir, nil)
	require.Error(t, err)

	unsupportedDir := filepath.Join(t.TempDir(), "unsupported")
	require.NoError(t, os.MkdirAll(unsupportedDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(unsupportedDir, "readme.xyz"), []byte("noop"), 0o644))
	bundle, err = BuildBuiltinCapabilityBundle(unsupportedDir, fakeCommandRunner{})
	require.NoError(t, err)
	require.NotNil(t, bundle.SearchEngine)

	require.True(t, shouldIgnoreBootstrapIndexError(errors.New("no parser for klingon")))
	require.False(t, shouldIgnoreBootstrapIndexError(nil))
}

func TestCapabilityBundleBuildsIndexByDefault(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main(){}\n"), 0o644))
	bundle, err := BuildBuiltinCapabilityBundle(dir, fakeCommandRunner{})
	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.NotNil(t, bundle.Registry)
	require.NotNil(t, bundle.IndexManager)
	require.NotNil(t, bundle.SearchEngine)
}

func TestCapabilityBundleWithPermissionManager(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main(){}\n"), 0o644))
	pm := mustOpenTestPermissionManager(t, dir)
	bundle, err := BuildBuiltinCapabilityBundle(dir, fakeCommandRunner{}, CapabilityRegistryOptions{
		AgentID:           "agent-1",
		PermissionManager: pm,
		AgentSpec: &core.AgentRuntimeSpec{
			Implementation: "react",
			Mode:           core.AgentModePrimary,
			Model:          core.AgentModelConfig{Name: "bundle-model"},
		},
		ProtectedPaths: []string{filepath.Join(dir, "config")},
	})
	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.NotNil(t, bundle.Registry)
	require.NotNil(t, bundle.IndexManager)
	require.NotNil(t, bundle.SearchEngine)
}

func TestCapabilityBundleCleansUpOnError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main(){}\n"), 0o644))

	t.Run("graphdb failure cleans store", func(t *testing.T) {
		origCleanup := cleanupCapabilityBundleFn
		origNewGraphDBFn := newGraphDBFn
		origNewASTSQLiteStoreFn := newASTSQLiteStoreFn
		t.Cleanup(func() {
			cleanupCapabilityBundleFn = origCleanup
			newGraphDBFn = origNewGraphDBFn
			newASTSQLiteStoreFn = origNewASTSQLiteStoreFn
		})

		var sawStore bool
		var sawManager bool
		cleanupCapabilityBundleFn = func(store *ast.SQLiteStore, manager *ast.IndexManager) {
			sawStore = store != nil
			sawManager = manager != nil
			origCleanup(store, manager)
		}
		newGraphDBFn = func(graphdb.Options) (*graphdb.Engine, error) {
			return nil, errors.New("graphdb boom")
		}

		_, err := BuildBuiltinCapabilityBundle(dir, fakeCommandRunner{})
		require.Error(t, err)
		require.True(t, sawStore)
		require.True(t, sawManager)
	})

	t.Run("post-acquisition failure cleans manager", func(t *testing.T) {
		origCleanup := cleanupCapabilityBundleFn
		origStartIndexingFn := startIndexingFn
		t.Cleanup(func() {
			cleanupCapabilityBundleFn = origCleanup
			startIndexingFn = origStartIndexingFn
		})

		var sawStore bool
		var sawManager bool
		cleanupCapabilityBundleFn = func(store *ast.SQLiteStore, manager *ast.IndexManager) {
			sawStore = store != nil
			sawManager = manager != nil
			origCleanup(store, manager)
		}
		startIndexingFn = func(*ast.IndexManager, context.Context) error {
			return errors.New("start boom")
		}

		_, err := BuildBuiltinCapabilityBundle(dir, fakeCommandRunner{})
		require.Error(t, err)
		require.True(t, sawStore)
		require.True(t, sawManager)
	})
}

func TestCapabilityBundleSeamBranches(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main(){}\n"), 0o644))

	cases := []struct {
		name string
		apply func()
		want  string
	}{
		{
			name: "ast store fail",
			apply: func() {
				newASTSQLiteStoreFn = func(string) (*ast.SQLiteStore, error) {
					return nil, errors.New("ast boom")
				}
			},
			want: "ast boom",
		},
		{
			name: "graphdb fail",
			apply: func() {
				newGraphDBFn = func(graphdb.Options) (*graphdb.Engine, error) {
					return nil, errors.New("graphdb boom")
				}
			},
			want: "graphdb boom",
		},
		{
			name: "code index fail",
			apply: func() {
				newCodeIndexFn = func(string, string) (*memory.CodeIndex, error) {
					return nil, errors.New("code boom")
				}
			},
			want: "code boom",
		},
		{
			name: "build ignore",
			apply: func() {
				buildCodeIndexFn = func(*memory.CodeIndex, context.Context) error {
					return errors.New("no parser for unknown")
				}
			},
		},
		{
			name: "build fail",
			apply: func() {
				buildCodeIndexFn = func(*memory.CodeIndex, context.Context) error {
					return errors.New("build boom")
				}
			},
			want: "build boom",
		},
		{
			name: "save fail",
			apply: func() {
				saveCodeIndexFn = func(*memory.CodeIndex) error {
					return errors.New("save boom")
				}
			},
			want: "save boom",
		},
		{
			name: "start fail",
			apply: func() {
				startIndexingFn = func(*ast.IndexManager, context.Context) error {
					return errors.New("start boom")
				}
			},
			want: "start boom",
		},
	{
			name: "search nil",
			apply: func() {
				newSearchEngineFn = func(search.SemanticStore, search.CodeIndex) *search.SearchEngine {
					return nil
				}
			},
			want: "search engine initialization failed",
		},
		{
			name: "duplicate tool",
			apply: func() {
				newSemanticSearchToolFn = func(workspace string) core.Tool {
					return newSimilarityToolFn(workspace)
				}
			},
			want: "already registered",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			origBuildCodeIndexFn := buildCodeIndexFn
			origSaveCodeIndexFn := saveCodeIndexFn
			origStartIndexingFn := startIndexingFn
			origNewGraphDBFn := newGraphDBFn
			origNewSearchEngineFn := newSearchEngineFn
			origNewASTSQLiteStoreFn := newASTSQLiteStoreFn
			origNewCodeIndexFn := newCodeIndexFn
			origNewSemanticSearchToolFn := newSemanticSearchToolFn
			t.Cleanup(func() {
				buildCodeIndexFn = origBuildCodeIndexFn
				saveCodeIndexFn = origSaveCodeIndexFn
				startIndexingFn = origStartIndexingFn
				newGraphDBFn = origNewGraphDBFn
				newSearchEngineFn = origNewSearchEngineFn
				newASTSQLiteStoreFn = origNewASTSQLiteStoreFn
				newCodeIndexFn = origNewCodeIndexFn
				newSemanticSearchToolFn = origNewSemanticSearchToolFn
			})
			tc.apply()

			bundle, err := BuildBuiltinCapabilityBundle(dir, fakeCommandRunner{})
			if tc.want == "" {
				require.NoError(t, err)
				require.NotNil(t, bundle)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}
