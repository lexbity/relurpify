package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/agents"
	runtimesvc "github.com/lexcodex/relurpify/app/relurpish/runtime"
	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/capabilityplan"
	"github.com/lexcodex/relurpify/framework/config"
	contractpkg "github.com/lexcodex/relurpify/framework/contract"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/policybundle"
	"github.com/lexcodex/relurpify/platform/llm"
	"github.com/stretchr/testify/require"
)

func TestRuntimeAdapterSessionInfoUsesLiveAgentModeAndStrategy(t *testing.T) {
	rt := &runtimesvc.Runtime{
		Config: runtimesvc.Config{
			Workspace:         "/workspace",
			InferenceProvider: "ollama",
			InferenceModel:    "base-model",
			AgentName:         "coding-go",
		},
		Agent: &agents.ReActAgent{},
		Registration: &fauthorization.AgentRegistration{
			Manifest: &manifest.AgentManifest{
				Metadata: manifest.ManifestMetadata{Name: "coding-go"},
				Spec: manifest.ManifestSpec{
					Agent: &core.AgentRuntimeSpec{
						Model: core.AgentModelConfig{Name: "manifest-model"},
						Mode:  core.AgentModePrimary,
						Context: core.AgentContextSpec{
							MaxTokens: 4096,
						},
					},
				},
			},
		},
	}

	info := (&runtimeAdapter{rt: rt}).SessionInfo()

	require.Equal(t, "coding-go", info.Agent)
	require.Equal(t, "ollama", info.Provider)
	require.Equal(t, "manifest-model", info.Model)
	require.Equal(t, "primary", info.Role)
	require.Equal(t, "react", info.Mode)
	require.Equal(t, "react", info.Strategy)
	require.Equal(t, 4096, info.MaxTokens)
}

func TestDescribeAgentRuntimeForReflectionUsesDelegateMode(t *testing.T) {
	mode, strategy := describeAgentRuntime(&agents.ReflectionAgent{
		Delegate: &agents.ReActAgent{},
	})

	require.Equal(t, "react", mode)
	require.Equal(t, "reflection", strategy)
}

type runtimeAdapterModelBackend struct {
	models []llm.ModelInfo
}

func (b runtimeAdapterModelBackend) Model() core.LanguageModel { return nil }
func (b runtimeAdapterModelBackend) Embedder() llm.Embedder    { return nil }
func (b runtimeAdapterModelBackend) Capabilities() core.BackendCapabilities {
	return core.BackendCapabilities{}
}
func (b runtimeAdapterModelBackend) Health(context.Context) (*llm.HealthReport, error) {
	return &llm.HealthReport{State: llm.BackendHealthReady}, nil
}
func (b runtimeAdapterModelBackend) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return append([]llm.ModelInfo(nil), b.models...), nil
}
func (b runtimeAdapterModelBackend) Warm(context.Context) error { return nil }
func (b runtimeAdapterModelBackend) Close() error               { return nil }
func (b runtimeAdapterModelBackend) SetDebugLogging(bool)       {}

func TestRuntimeAdapterInferenceModelsListsBackendModels(t *testing.T) {
	adapter := &runtimeAdapter{rt: &runtimesvc.Runtime{
		Backend: runtimeAdapterModelBackend{
			models: []llm.ModelInfo{{Name: "model-a"}, {Name: "model-b"}},
		},
	}}
	models, err := adapter.InferenceModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 2)
	require.Equal(t, "model-a", models[0])
	require.Equal(t, "model-b", models[1])
}

func TestRuntimeAdapterSaveModelPersistsProviderAndModel(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "relurpify.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("provider: anthropic\nmodel: old-model\n"), 0o644))
	adapter := &runtimeAdapter{rt: &runtimesvc.Runtime{
		Config: runtimesvc.Config{
			Workspace:         dir,
			ConfigPath:        cfgPath,
			InferenceProvider: "ollama",
			InferenceModel:    "base-model",
		},
	}}
	require.NoError(t, adapter.SaveModel("selected-model"))
	saved, err := runtimesvc.LoadWorkspaceConfig(cfgPath)
	require.NoError(t, err)
	require.Equal(t, "ollama", saved.Provider)
	require.Equal(t, "selected-model", saved.Model)
}

func TestRuntimeAdapterListsWorkflows(t *testing.T) {
	workspace := t.TempDir()
	dbPath := config.New(workspace).WorkflowStateFile()
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	store, err := db.NewSQLiteWorkflowStateStore(dbPath)
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "wf-1",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "Inspect me",
		Status:      memory.WorkflowRunStatusRunning,
		UpdatedAt:   time.Now().UTC(),
	}))
	require.NoError(t, store.CreateRun(context.Background(), memory.WorkflowRunRecord{
		RunID:      "run-1",
		WorkflowID: "wf-1",
		Status:     memory.WorkflowRunStatusRunning,
	}))
	require.NoError(t, store.UpsertDelegation(context.Background(), memory.WorkflowDelegationRecord{
		DelegationID:   "delegation-1",
		WorkflowID:     "wf-1",
		RunID:          "run-1",
		TaskID:         "wf-1",
		State:          core.DelegationStateSucceeded,
		TrustClass:     core.TrustClassBuiltinTrusted,
		Recoverability: core.RecoverabilityInProcess,
		Request: core.DelegationRequest{
			ID:                 "delegation-1",
			WorkflowID:         "wf-1",
			TaskID:             "wf-1",
			TargetCapabilityID: "relurpic:planner.plan",
			TaskType:           "plan",
			Instruction:        "Inspect me",
			ResourceRefs:       []string{"workflow://wf-1/warm?run=run-1&role=planner"},
		},
		Result: &core.DelegationResult{
			DelegationID: "delegation-1",
			State:        core.DelegationStateSucceeded,
			Success:      true,
			Insertion:    core.InsertionDecision{Action: core.InsertionActionSummarized},
		},
		StartedAt: time.Now().UTC().Add(-time.Minute),
		UpdatedAt: time.Now().UTC(),
	}))
	require.NoError(t, store.AppendDelegationTransition(context.Background(), memory.WorkflowDelegationTransitionRecord{
		TransitionID: "delegation-1:succeeded",
		DelegationID: "delegation-1",
		WorkflowID:   "wf-1",
		RunID:        "run-1",
		ToState:      core.DelegationStateSucceeded,
		CreatedAt:    time.Now().UTC(),
	}))
	require.NoError(t, store.UpsertWorkflowArtifact(context.Background(), memory.WorkflowArtifactRecord{
		ArtifactID:    "artifact-1",
		WorkflowID:    "wf-1",
		RunID:         "run-1",
		Kind:          "delegation_result",
		ContentType:   "application/json",
		StorageKind:   memory.ArtifactStorageInline,
		SummaryText:   "delegation summary",
		InlineRawText: `{"summary":"delegated"}`,
		RawSizeBytes:  int64(len(`{"summary":"delegated"}`)),
	}))
	require.NoError(t, store.ReplaceProviderSnapshots(context.Background(), "wf-1", "run-1", []memory.WorkflowProviderSnapshotRecord{{
		SnapshotID:     "provider-1",
		WorkflowID:     "wf-1",
		RunID:          "run-1",
		ProviderID:     "delegation-runtime",
		Recoverability: core.RecoverabilityInProcess,
		Descriptor:     core.ProviderDescriptor{ID: "delegation-runtime", Kind: core.ProviderKindAgentRuntime},
		Health:         core.ProviderHealthSnapshot{Status: "ok"},
	}}))
	require.NoError(t, store.ReplaceProviderSessionSnapshots(context.Background(), "wf-1", "run-1", []memory.WorkflowProviderSessionSnapshotRecord{{
		SnapshotID: "session-1",
		WorkflowID: "wf-1",
		RunID:      "run-1",
		Session: core.ProviderSession{
			ID:             "session-1",
			ProviderID:     "delegation-runtime",
			Recoverability: core.RecoverabilityInProcess,
			Health:         "running",
		},
		CapturedAt: time.Now().UTC(),
	}}))

	rt := &runtimesvc.Runtime{
		Config: runtimesvc.Config{
			Workspace: workspace,
		},
	}

	adapter := &runtimeAdapter{rt: rt}
	workflows, err := adapter.ListWorkflows(10)
	require.NoError(t, err)
	require.Len(t, workflows, 1)
	require.Equal(t, "wf-1", workflows[0].WorkflowID)

	details, err := adapter.GetWorkflow("wf-1")
	require.NoError(t, err)
	require.Equal(t, "wf-1", details.Workflow.WorkflowID)
	require.Len(t, details.Delegations, 1)
	require.Equal(t, "delegation-1", details.Delegations[0].DelegationID)
	require.Len(t, details.Transitions, 1)
	require.Len(t, details.WorkflowArtifacts, 1)
	require.Len(t, details.Providers, 1)
	require.Len(t, details.ProviderSessions, 1)
	require.Equal(t, []string{"workflow://wf-1/warm?run=run-1&role=planner"}, details.LinkedResources)
}

func TestRuntimeAdapterListCapabilitiesIncludesRuntimeFamily(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(relurpicCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "relurpic:planner.plan",
			Name:          "planner.plan",
			Kind:          core.CapabilityKindPrompt,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
		},
	}))
	require.NoError(t, registry.RegisterInvocableCapability(relurpicCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "provider:browser",
			Name:          "browser",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
			Source: core.CapabilitySource{
				Scope:      core.CapabilityScopeProvider,
				ProviderID: "browser",
			},
			TrustClass:   core.TrustClassProviderLocalUntrusted,
			Availability: core.AvailabilitySpec{Available: true},
		},
	}))
	registry.UseAgentSpec("agent", &core.AgentRuntimeSpec{
		Mode:  core.AgentModePrimary,
		Model: core.AgentModelConfig{Provider: "test", Name: "test"},
		ExposurePolicies: []core.CapabilityExposurePolicy{{
			Selector: core.CapabilitySelector{Name: "browser", RuntimeFamilies: []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyProvider}},
			Access:   core.CapabilityExposureCallable,
		}},
	})

	adapter := &runtimeAdapter{rt: &runtimesvc.Runtime{Tools: registry}}
	capabilities := adapter.ListCapabilities()
	require.Len(t, capabilities, 2)
	byName := make(map[string]CapabilityInfo, len(capabilities))
	for _, capability := range capabilities {
		byName[capability.Name] = capability
	}
	require.Equal(t, "provider", byName["browser"].RuntimeFamily)
	require.True(t, byName["browser"].Callable)
	require.Equal(t, "relurpic", byName["planner.plan"].RuntimeFamily)
	require.True(t, byName["planner.plan"].Callable)
}

func TestRuntimeAdapterListToolsInfoReportsLocalToolRuntimeFamily(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(localToolStub{name: "file_read"}))

	adapter := &runtimeAdapter{rt: &runtimesvc.Runtime{Tools: registry}}
	tools := adapter.ListToolsInfo()
	require.Len(t, tools, 1)
	require.Equal(t, "local-tool", tools[0].RuntimeFamily)
	require.Equal(t, "builtin", tools[0].Scope)
}

func TestRuntimeAdapterExposesContractSummaryAndAdmissions(t *testing.T) {
	adapter := &runtimeAdapter{rt: &runtimesvc.Runtime{
		Tools: capability.NewRegistry(),
		EffectiveContract: &contractpkg.EffectiveAgentContract{
			AgentID: "coding",
			Sources: contractpkg.SourceSummary{
				ManifestName:    "coding",
				ManifestVersion: "1.2.3",
				Workspace:       "/tmp/ws",
				AppliedSkills:   []string{"reviewer"},
				FailedSkills:    []string{"broken-skill"},
			},
		},
		CompiledPolicy: &policybundle.CompiledPolicyBundle{
			AgentID: "coding",
			Rules: []core.PolicyRule{
				{ID: "rule-1", Name: "rule-1"},
			},
		},
		CapabilityAdmissions: []capabilityplan.AdmissionResult{
			{CapabilityID: "prompt:reviewer:1", CapabilityName: "reviewer.prompt.1", Kind: core.CapabilityKindPrompt, Admitted: true, Reason: "admitted"},
			{CapabilityID: "resource:broken:1", CapabilityName: "broken.resource.1", Kind: core.CapabilityKindResource, Admitted: false, Reason: "filtered by allowed capabilities"},
		},
	}}

	summary := adapter.ContractSummary()
	require.NotNil(t, summary)
	require.Equal(t, "coding", summary.AgentID)
	require.Equal(t, "1.2.3", summary.ManifestVersion)
	require.Equal(t, 2, summary.AdmissionCount)
	require.Equal(t, 1, summary.RejectedCount)
	require.Equal(t, 1, summary.PolicyRuleCount)
	require.Equal(t, []string{"reviewer"}, summary.AppliedSkills)
	require.Equal(t, []string{"broken-skill"}, summary.FailedSkills)

	admissions := adapter.CapabilityAdmissions()
	require.Len(t, admissions, 2)
	require.Equal(t, "prompt:reviewer:1", admissions[0].CapabilityID)
	require.False(t, admissions[1].Admitted)
	require.Equal(t, "filtered by allowed capabilities", admissions[1].Reason)
}

type relurpicCapabilityStub struct {
	desc core.CapabilityDescriptor
}

type adapterLiveProvider struct {
	desc     core.ProviderDescriptor
	sessions []core.ProviderSession
}

func (p *adapterLiveProvider) Initialize(context.Context, *runtimesvc.Runtime) error { return nil }
func (p *adapterLiveProvider) Close() error                                          { return nil }
func (p *adapterLiveProvider) Descriptor() core.ProviderDescriptor                   { return p.desc }
func (p *adapterLiveProvider) ListSessions(context.Context) ([]core.ProviderSession, error) {
	return append([]core.ProviderSession(nil), p.sessions...), nil
}
func (p *adapterLiveProvider) HealthSnapshot(context.Context) (core.ProviderHealthSnapshot, error) {
	return core.ProviderHealthSnapshot{Status: "ok"}, nil
}

func (c relurpicCapabilityStub) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return c.desc
}

func TestRuntimeAdapterListsLiveProvidersSessionsAndApprovals(t *testing.T) {
	rt := &runtimesvc.Runtime{
		AgentSpec: &core.AgentRuntimeSpec{
			ProviderPolicies: map[string]core.ProviderPolicy{
				"remote-mcp": {Activate: core.AgentPermissionAllow},
			},
		},
		Registration: &fauthorization.AgentRegistration{
			HITL: fauthorization.NewHITLBroker(time.Minute),
		},
	}
	provider := &adapterLiveProvider{
		desc: core.ProviderDescriptor{
			ID:                 "remote-mcp",
			Kind:               core.ProviderKindMCPClient,
			ConfiguredSource:   "stdio://fixture",
			TrustBaseline:      core.TrustClassRemoteDeclared,
			RecoverabilityMode: core.RecoverabilityPersistedRestore,
			Security: core.ProviderSecurityProfile{
				Origin: core.ProviderOriginRemote,
			},
		},
		sessions: []core.ProviderSession{{
			ID:             "remote-mcp:primary",
			ProviderID:     "remote-mcp",
			TrustClass:     core.TrustClassRemoteDeclared,
			Recoverability: core.RecoverabilityPersistedRestore,
			Health:         "running",
			Metadata: map[string]interface{}{
				"protocol_version": "2025-06-18",
			},
		}},
	}
	require.NoError(t, rt.RegisterProvider(context.Background(), provider))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = rt.Registration.HITL.RequestPermission(ctx, fauthorization.PermissionRequest{
			Permission: core.PermissionDescriptor{
				Type:         core.PermissionTypeCapability,
				Action:       "provider:remote-mcp:activate",
				Resource:     "remote-mcp",
				Metadata:     map[string]string{"provider_id": "remote-mcp"},
				RequiresHITL: true,
			},
			Justification: "activate provider remote-mcp",
			Scope:         fauthorization.GrantScopeSession,
			Risk:          fauthorization.RiskLevelMedium,
		})
	}()
	require.Eventually(t, func() bool {
		return len(rt.Registration.HITL.PendingRequests()) == 1
	}, time.Second, 10*time.Millisecond)

	adapter := &runtimeAdapter{rt: rt}
	providers := adapter.ListLiveProviders()
	require.Len(t, providers, 1)
	require.Equal(t, "remote-mcp", providers[0].ProviderID)
	require.Equal(t, "mcp-client", providers[0].Kind)

	sessions := adapter.ListLiveSessions()
	require.Len(t, sessions, 1)
	require.Equal(t, "remote-mcp:primary", sessions[0].SessionID)
	require.Contains(t, sessions[0].MetadataSummary, "protocol_version=2025-06-18")

	approvals := adapter.ListApprovals()
	require.Len(t, approvals, 1)
	require.Equal(t, "provider_operation", approvals[0].Kind)
	require.Equal(t, "provider:remote-mcp:activate", approvals[0].Action)

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for approval request goroutine to exit")
	}
}

func (c relurpicCapabilityStub) Invoke(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}

type localToolStub struct {
	name string
}

func (t localToolStub) Name() string        { return t.name }
func (t localToolStub) Description() string { return t.name }
func (t localToolStub) Category() string    { return "test" }
func (t localToolStub) Parameters() []core.ToolParameter {
	return nil
}
func (t localToolStub) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (t localToolStub) IsAvailable(context.Context, *core.Context) bool { return true }
func (t localToolStub) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (t localToolStub) Tags() []string                                  { return []string{core.TagReadOnly} }
