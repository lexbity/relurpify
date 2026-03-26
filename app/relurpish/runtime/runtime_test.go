package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	relurpic "github.com/lexcodex/relurpify/agents/relurpic"
	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/config"
	contract "github.com/lexcodex/relurpify/framework/contract"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
	frameworksearch "github.com/lexcodex/relurpify/framework/search"
	"github.com/lexcodex/relurpify/named/euclo"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestProbeEnvironmentHandlesMissingRunsc surfaces a helpful error message.
func TestProbeEnvironmentHandlesMissingRunsc(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Workspace = dir
	cfg.ManifestPath = filepath.Join(dir, "agent.manifest.yaml")
	cfg.ConfigPath = config.New(dir).ConfigFile()
	cfg.Sandbox.RunscPath = "runsc-missing"
	report := ProbeEnvironment(context.Background(), cfg)
	require.Contains(t, strings.Join(report.Sandbox.Errors, " "), "runsc not found")
}

func TestSummarizeManifestMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.manifest.yaml")
	summary := summarizeManifest(path)
	require.Equal(t, path, summary.Path)
	require.False(t, summary.Exists)
	require.Empty(t, summary.Error)
}

func TestInitializeWorkspaceFromTemplatesCreatesWorkspaceFiles(t *testing.T) {
	shared := t.TempDir()
	t.Setenv("RELURPIFY_SHARED_DIR", shared)
	configTemplate := filepath.Join(shared, "templates", "workspace", "config.yaml")
	manifestTemplate := filepath.Join(shared, "templates", "workspace", "agent.manifest.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configTemplate), 0o755))
	require.NoError(t, os.WriteFile(configTemplate, []byte("model: test-model\n"), 0o644))
	require.NoError(t, os.WriteFile(manifestTemplate, []byte("path: ${workspace}\n"), 0o644))

	dir := t.TempDir()
	cfg := Config{Workspace: dir}
	require.NoError(t, cfg.Normalize())

	require.NoError(t, InitializeWorkspaceFromTemplates(cfg, false))
	data, err := os.ReadFile(cfg.ConfigPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "test-model")
	manifestData, err := os.ReadFile(cfg.ManifestPath)
	require.NoError(t, err)
	require.Contains(t, string(manifestData), filepath.ToSlash(dir))
	for _, path := range []string{
		config.New(dir).AgentsDir(),
		config.New(dir).SkillsDir(),
		config.New(dir).LogsDir(),
		config.New(dir).TelemetryDir(),
		config.New(dir).MemoryDir(),
		config.New(dir).SessionsDir(),
		config.New(dir).TestRunsDir(),
	} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.True(t, info.IsDir())
	}
}

func TestInitializeWorkspaceFromTemplatesDoesNotOverwriteWithoutFix(t *testing.T) {
	shared := t.TempDir()
	t.Setenv("RELURPIFY_SHARED_DIR", shared)
	configTemplate := filepath.Join(shared, "templates", "workspace", "config.yaml")
	manifestTemplate := filepath.Join(shared, "templates", "workspace", "agent.manifest.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(configTemplate), 0o755))
	require.NoError(t, os.WriteFile(configTemplate, []byte("model: replacement\n"), 0o644))
	require.NoError(t, os.WriteFile(manifestTemplate, []byte("name: replacement\n"), 0o644))

	dir := t.TempDir()
	cfg := Config{Workspace: dir}
	require.NoError(t, cfg.Normalize())
	require.NoError(t, os.MkdirAll(filepath.Dir(cfg.ConfigPath), 0o755))
	require.NoError(t, os.WriteFile(cfg.ConfigPath, []byte("model: keep\n"), 0o644))
	require.NoError(t, os.WriteFile(cfg.ManifestPath, []byte("name: keep\n"), 0o644))

	require.NoError(t, InitializeWorkspaceFromTemplates(cfg, false))
	configData, err := os.ReadFile(cfg.ConfigPath)
	require.NoError(t, err)
	require.Contains(t, string(configData), "keep")
	manifestData, err := os.ReadFile(cfg.ManifestPath)
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "keep")
}

func TestDetectChromiumStatusMissingIsWarningOnly(t *testing.T) {
	orig := execLookPath
	execLookPath = func(file string) (string, error) {
		return "", os.ErrNotExist
	}
	defer func() { execLookPath = orig }()

	status := detectChromiumStatus(context.Background())
	require.Equal(t, "chromium", status.Name)
	require.False(t, status.Required)
	require.False(t, status.Available)
	require.False(t, status.Blocking)
	require.Equal(t, "not found", status.Details)
}

func TestBuildCapabilityRegistryDoesNotRegisterGenericExecutionToolsByDefault(t *testing.T) {
	dir := t.TempDir()
	runner := fsandbox.NewLocalCommandRunner(dir, nil)

	capabilities, err := BuildBuiltinCapabilityBundle(dir, runner)
	require.NoError(t, err)
	registry := capabilities.Registry
	indexManager := capabilities.IndexManager
	searchEngine := capabilities.SearchEngine
	require.NotNil(t, registry)
	require.NotNil(t, indexManager)
	require.NotNil(t, indexManager.GraphDB)
	require.NotNil(t, searchEngine)

	_, ok := registry.Get("exec_run_tests")
	require.False(t, ok)
	_, ok = registry.Get("exec_run_linter")
	require.False(t, ok)
	_, ok = registry.Get("exec_run_build")
	require.False(t, ok)
	_, ok = registry.Get("exec_run_code")
	require.False(t, ok)
	_, ok = registry.Get("search_grep")
	require.False(t, ok)

	_, ok = registry.Get("go_test")
	require.True(t, ok)
	_, ok = registry.Get("file_search")
	require.True(t, ok)
}

func TestBuildCapabilityRegistryReturnsUsableSearchEngine(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "match.go"), []byte("package sample\nfunc Hello() string { return \"needle\" }\n"), 0o644))
	runner := fsandbox.NewLocalCommandRunner(dir, nil)

	capabilities, err := BuildBuiltinCapabilityBundle(dir, runner)
	require.NoError(t, err)
	searchEngine := capabilities.SearchEngine
	require.NotNil(t, searchEngine)

	results, err := searchEngine.Search(frameworksearch.SearchQuery{
		Text:       "needle",
		Mode:       frameworksearch.SearchKeyword,
		MaxResults: 5,
	})
	require.NoError(t, err)
	require.NotEmpty(t, results)
	require.Contains(t, results[0].File, "match.go")
}

func TestBuildCapabilityRegistryContinuesWhenBootstrapContextCanceled(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "match.go"), []byte("package sample\nfunc Hello() string { return \"needle\" }\n"), 0o644))
	runner := fsandbox.NewLocalCommandRunner(dir, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	capabilities, err := BuildBuiltinCapabilityBundle(dir, runner, CapabilityRegistryOptions{Context: ctx})
	require.NoError(t, err)
	require.NotNil(t, capabilities)
	require.NotNil(t, capabilities.Registry)
	require.NotNil(t, capabilities.IndexManager)
	require.NotNil(t, capabilities.IndexManager.GraphDB)
	require.NotNil(t, capabilities.SearchEngine)
}

func TestBuildCapabilityRegistrySkipASTIndexSkipsSemanticBootstrap(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "match.go"), []byte("package sample\nfunc Hello() string { return \"needle\" }\n"), 0o644))
	runner := fsandbox.NewLocalCommandRunner(dir, nil)

	var embedCalls atomic.Int32
	embedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		embedCalls.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"embeddings":[[1,2]]}`))
	}))
	defer embedServer.Close()

	capabilities, err := BuildBuiltinCapabilityBundle(dir, runner, CapabilityRegistryOptions{
		OllamaEndpoint: embedServer.URL,
		OllamaModel:    "test-embedder",
		SkipASTIndex:   true,
	})
	require.NoError(t, err)
	require.NotNil(t, capabilities)
	require.NotNil(t, capabilities.IndexManager)
	require.NotNil(t, capabilities.IndexManager.GraphDB)
	require.NotNil(t, capabilities.SearchEngine)
	require.Zero(t, embedCalls.Load())

	results, err := capabilities.SearchEngine.Search(frameworksearch.SearchQuery{
		Text:       "needle",
		Mode:       frameworksearch.SearchKeyword,
		MaxResults: 5,
	})
	require.NoError(t, err)
	require.NotEmpty(t, results)
	require.Contains(t, results[0].File, "match.go")
}

func TestWireRuntimeAgentDependenciesConfiguresEucloAgent(t *testing.T) {
	dir := t.TempDir()
	graphEngine, err := graphdb.Open(graphdb.DefaultOptions(filepath.Join(dir, "graphdb")))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, graphEngine.Close())
	})

	workflowStore, planStore, patternStore, commentStore, patternDB, err := openRuntimeStores(dir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, workflowStore.Close())
		require.NoError(t, patternDB.Close())
	})

	rt := &Runtime{
		GraphDB:        graphEngine,
		PlanStore:      planStore,
		PatternStore:   patternStore,
		CommentStore:   commentStore,
		WorkflowStore:  workflowStore,
		GuidanceBroker: guidance.NewGuidanceBroker(time.Second),
	}
	agent := &euclo.Agent{}

	rt.wireRuntimeAgentDependencies(agent)

	require.Same(t, graphEngine, agent.GraphDB)
	require.Same(t, workflowStore.DB(), agent.RetrievalDB)
	require.Same(t, planStore, agent.PlanStore)
	require.Same(t, patternStore, agent.PatternStore)
	require.Same(t, commentStore, agent.CommentStore)
	require.Same(t, rt.GuidanceBroker, agent.GuidanceBroker)
	require.Equal(t, guidance.DefaultDeferralPolicy(), agent.DeferralPolicy)
	require.IsType(t, &relurpic.PatternCoherenceVerifier{}, agent.ConvVerifier)
}

func TestOpenRuntimeStoresReturnsSharedPersistenceSurfaces(t *testing.T) {
	dir := t.TempDir()

	workflowStore, planStore, patternStore, commentStore, patternDB, err := openRuntimeStores(dir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, workflowStore.Close())
		require.NoError(t, patternDB.Close())
	})

	require.NotNil(t, workflowStore)
	require.NotNil(t, workflowStore.DB())
	require.Implements(t, (*frameworkplan.PlanStore)(nil), planStore)
	require.Implements(t, (*patterns.PatternStore)(nil), patternStore)
	require.Implements(t, (*patterns.CommentStore)(nil), commentStore)
}

func TestReloadEffectiveContractRefreshesDefinitionOverlay(t *testing.T) {
	workspace := t.TempDir()
	agentsDir := filepath.Join(workspace, "agents")
	require.NoError(t, os.MkdirAll(agentsDir, 0o755))
	defPath := filepath.Join(agentsDir, "reviewer.yaml")
	writeAgentDefinitionFixture(t, defPath, core.AgentPermissionDeny)

	permMgr, err := authorization.NewPermissionManager(workspace, &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "**"}},
	}, nil, nil)
	require.NoError(t, err)
	rt := &Runtime{
		Config: Config{
			Workspace:      workspace,
			AgentsDir:      agentsDir,
			AgentName:      "reviewer",
			OllamaModel:    "manifest-model",
			OllamaEndpoint: "http://localhost:11434",
		},
		Tools: capability.NewRegistry(),
		Registration: &authorization.AgentRegistration{
			ID: "coding",
			Manifest: &manifest.AgentManifest{
				Metadata: manifest.ManifestMetadata{Name: "coding"},
				Spec: manifest.ManifestSpec{
					Agent: &core.AgentRuntimeSpec{
						Implementation: "react",
						Mode:           core.AgentModePrimary,
						Model:          core.AgentModelConfig{Provider: "ollama", Name: "manifest-model"},
					},
				},
			},
			Permissions: permMgr,
		},
		Context: core.NewContext(),
	}
	require.NoError(t, rt.ReloadEffectiveContract())
	require.Equal(t, core.AgentPermissionDeny, rt.EffectiveContract.AgentSpec.ProviderPolicies["browser"].Activate)

	writeAgentDefinitionFixture(t, defPath, core.AgentPermissionAllow)
	require.NoError(t, rt.ReloadEffectiveContract())
	require.Equal(t, core.AgentPermissionAllow, rt.EffectiveContract.AgentSpec.ProviderPolicies["browser"].Activate)
	require.NotNil(t, rt.CompiledPolicy)
}

func TestReloadEffectiveContractRejectsSkillTopologyChanges(t *testing.T) {
	workspace := t.TempDir()
	skillRoot := filepath.Join(workspace, "relurpify_cfg", "skills", "reviewer")
	for _, dir := range []string{"scripts", "resources", "templates"} {
		require.NoError(t, os.MkdirAll(filepath.Join(skillRoot, dir), 0o755))
	}
	skill := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: "reviewer", Version: "1.0.0"},
		Spec: manifest.SkillSpec{
			PromptSnippets: []string{"Review carefully."},
			AllowedCapabilities: []core.CapabilitySelector{{
				Name: "reviewer.prompt.1",
				Kind: core.CapabilityKindPrompt,
			}},
		},
	}
	data, err := yaml.Marshal(skill)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(skillRoot, "skill.manifest.yaml"), data, 0o644))

	permMgr, err := authorization.NewPermissionManager(workspace, &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "**"}},
	}, nil, nil)
	require.NoError(t, err)
	rt := &Runtime{
		Config: Config{
			Workspace:      workspace,
			AgentName:      "coding",
			OllamaModel:    "manifest-model",
			OllamaEndpoint: "http://localhost:11434",
		},
		Tools: capability.NewRegistry(),
		Registration: &authorization.AgentRegistration{
			ID: "coding",
			Manifest: &manifest.AgentManifest{
				Metadata: manifest.ManifestMetadata{Name: "coding"},
				Spec: manifest.ManifestSpec{
					Agent: &core.AgentRuntimeSpec{
						Implementation: "react",
						Mode:           core.AgentModePrimary,
						Model:          core.AgentModelConfig{Provider: "ollama", Name: "manifest-model"},
					},
				},
			},
			Permissions: permMgr,
		},
		Context: core.NewContext(),
		EffectiveContract: &contract.EffectiveAgentContract{
			AgentID: "coding",
			AgentSpec: &core.AgentRuntimeSpec{
				Implementation: "react",
				Mode:           core.AgentModePrimary,
				Model:          core.AgentModelConfig{Provider: "ollama", Name: "manifest-model"},
			},
		},
	}
	rt.Registration.Manifest.Spec.Skills = []string{"reviewer"}

	err = rt.ReloadEffectiveContract()
	require.Error(t, err)
	require.Contains(t, err.Error(), "skill capability topology")
}

func writeAgentDefinitionFixture(t *testing.T, path string, level core.AgentPermissionLevel) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(`
kind: AgentDefinition
name: reviewer
spec:
  implementation: react
  mode: primary
  model:
    provider: ollama
    name: manifest-model
  provider_policies:
    browser:
      activate: `+string(level)+`
`), 0o644))
}
