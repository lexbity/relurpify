package runtime

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	mstdio "github.com/lexcodex/relurpify/framework/middleware/mcp/transport/stdio"
	"github.com/stretchr/testify/require"
)

type delegationRecordingTelemetry struct {
	events []core.Event
}

func (t *delegationRecordingTelemetry) Emit(event core.Event) {
	t.events = append(t.events, event)
}

func TestRuntimeDelegationWrappers(t *testing.T) {
	rt := &Runtime{
		Delegations: fauthorization.NewDelegationManager(),
	}

	started, err := rt.StartDelegation(context.Background(), core.DelegationRequest{
		ID:                 "delegation-1",
		WorkflowID:         "workflow-1",
		TargetCapabilityID: "agent:planner",
		TaskType:           "plan",
		Instruction:        "Create a plan",
	}, fauthorization.DelegationStartOptions{})
	require.NoError(t, err)
	require.Equal(t, core.DelegationStateRunning, started.State)

	listed := rt.ListDelegations(core.DelegationFilter{WorkflowID: "workflow-1"})
	require.Len(t, listed, 1)

	completed, err := rt.CompleteDelegation("delegation-1", &core.DelegationResult{
		DelegationID: "delegation-1",
		State:        core.DelegationStateSucceeded,
		Success:      true,
	})
	require.NoError(t, err)
	require.Equal(t, core.DelegationStateSucceeded, completed.State)
}

func TestRuntimePersistDelegations(t *testing.T) {
	rt := &Runtime{
		Delegations: fauthorization.NewDelegationManager(),
	}
	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow_state.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "workflow-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "persist runtime delegations",
	}))
	require.NoError(t, store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-1",
		WorkflowID: "workflow-1",
		Status:     memory.WorkflowRunStatusRunning,
	}))
	_, err = rt.StartDelegation(ctx, core.DelegationRequest{
		ID:                 "delegation-1",
		WorkflowID:         "workflow-1",
		TaskID:             "task-1",
		TargetCapabilityID: "agent:planner",
		TaskType:           "plan",
		Instruction:        "Create a plan",
	}, fauthorization.DelegationStartOptions{})
	require.NoError(t, err)

	require.NoError(t, rt.PersistDelegations(ctx, store, "workflow-1", "run-1"))
	records, err := store.ListDelegations(ctx, "workflow-1", "run-1")
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "delegation-1", records[0].DelegationID)
}

func TestRuntimeExecuteDelegationUsesRuntimeRegistryAndContext(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(runtimeDelegationCapability{
		desc: core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
			ID:            "relurpic:planner.plan",
			Name:          "planner.plan",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
			Coordination: &core.CoordinationTargetMetadata{
				Target:         true,
				Role:           core.CoordinationRolePlanner,
				TaskTypes:      []string{"plan"},
				ExecutionModes: []core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
			},
			InputSchema: &core.Schema{
				Type: "object",
				Properties: map[string]*core.Schema{
					"instruction": {Type: "string"},
				},
				Required: []string{"instruction"},
			},
			OutputSchema: &core.Schema{
				Type: "object",
				Properties: map[string]*core.Schema{
					"summary": {Type: "string"},
				},
				Required: []string{"summary"},
			},
		}),
	}))
	rt := &Runtime{
		Config:      Config{Workspace: t.TempDir()},
		Tools:       registry,
		Context:     core.NewContext(),
		Delegations: fauthorization.NewDelegationManager(),
		AgentSpec: &core.AgentRuntimeSpec{
			Mode:  core.AgentModePrimary,
			Model: core.AgentModelConfig{Name: "stub", Provider: "test"},
			Coordination: core.AgentCoordinationSpec{
				Enabled: true,
				DelegationTargetSelectors: []core.CapabilitySelector{{
					CoordinationRoles:     []core.CoordinationRole{core.CoordinationRolePlanner},
					CoordinationTaskTypes: []string{"plan"},
				}},
			},
		},
	}
	rt.Context.Set("runtime.active", true)

	snapshot, err := rt.ExecuteDelegation(context.Background(), core.DelegationRequest{
		ID:          "delegation-rt-1",
		TaskType:    "plan",
		Instruction: "Plan runtime work",
	}, fauthorization.DelegationExecutionOptions{})
	require.NoError(t, err)
	require.Equal(t, core.DelegationStateSucceeded, snapshot.State)
	require.Equal(t, "relurpic:planner.plan", snapshot.Request.TargetCapabilityID)
	require.Equal(t, core.InsertionActionSummarized, snapshot.Result.Insertion.Action)
}

func TestRuntimeExecuteDelegationBackgroundUsesProviderBackedSession(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(runtimeDelegationCapability{
		desc: core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
			ID:            "relurpic:architect.execute",
			Name:          "architect.execute",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
			Coordination: &core.CoordinationTargetMetadata{
				Target:    true,
				Role:      core.CoordinationRoleArchitect,
				TaskTypes: []string{"implement"},
				ExecutionModes: []core.CoordinationExecutionMode{
					core.CoordinationExecutionModeBackgroundAgent,
				},
			},
			InputSchema: &core.Schema{
				Type: "object",
				Properties: map[string]*core.Schema{
					"instruction": {Type: "string"},
				},
				Required: []string{"instruction"},
			},
			OutputSchema: &core.Schema{
				Type: "object",
				Properties: map[string]*core.Schema{
					"summary": {Type: "string"},
				},
				Required: []string{"summary"},
			},
		}),
		delay: 25 * time.Millisecond,
	}))
	rt := &Runtime{
		Config:      Config{Workspace: t.TempDir()},
		Tools:       registry,
		Context:     core.NewContext(),
		Delegations: fauthorization.NewDelegationManager(),
		AgentSpec: &core.AgentRuntimeSpec{
			Mode:  core.AgentModePrimary,
			Model: core.AgentModelConfig{Name: "stub", Provider: "test"},
			Coordination: core.AgentCoordinationSpec{
				Enabled:                   true,
				AllowBackgroundDelegation: true,
				DelegationTargetSelectors: []core.CapabilitySelector{{
					CoordinationRoles:     []core.CoordinationRole{core.CoordinationRoleArchitect},
					CoordinationTaskTypes: []string{"implement"},
				}},
			},
			ProviderPolicies: map[string]core.ProviderPolicy{
				backgroundDelegationProviderID: {Activate: core.AgentPermissionAllow},
			},
		},
	}

	snapshot, err := rt.ExecuteDelegation(context.Background(), core.DelegationRequest{
		ID:          "delegation-rt-bg-1",
		TaskType:    "implement",
		Instruction: "Implement asynchronously",
		Metadata: map[string]any{
			"background": true,
		},
	}, fauthorization.DelegationExecutionOptions{})
	require.NoError(t, err)
	require.Equal(t, core.DelegationStateRunning, snapshot.State)
	require.Equal(t, backgroundDelegationProviderID, snapshot.Request.TargetProviderID)
	require.NotEmpty(t, snapshot.Request.TargetSessionID)

	providers, sessions, err := rt.CaptureProviderSnapshots(context.Background())
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.Equal(t, backgroundDelegationProviderID, providers[0].ProviderID)
	require.Len(t, sessions, 1)
	require.Equal(t, snapshot.Request.TargetSessionID, sessions[0].Session.ID)

	require.Eventually(t, func() bool {
		current, err := rt.Delegations.GetDelegation("delegation-rt-bg-1")
		return err == nil && current.State == core.DelegationStateSucceeded
	}, time.Second, 10*time.Millisecond)
}

func TestRuntimeExecuteDelegationUsesImportedMCPRemoteCoordinationService(t *testing.T) {
	server, launcher := newFixtureMCPServer()
	prevFactory := mcpClientLauncherFactory
	mcpClientLauncherFactory = func(core.ProviderConfig) mstdio.Launcher { return launcher }
	defer func() { mcpClientLauncherFactory = prevFactory }()

	registry := capability.NewRegistry()
	rt := &Runtime{
		Config:      Config{Workspace: t.TempDir()},
		Tools:       registry,
		Context:     core.NewContext(),
		Delegations: fauthorization.NewDelegationManager(),
		AgentSpec: &core.AgentRuntimeSpec{
			Mode:  core.AgentModePrimary,
			Model: core.AgentModelConfig{Name: "stub", Provider: "test"},
			Coordination: core.AgentCoordinationSpec{
				Enabled:               true,
				AllowRemoteDelegation: true,
				DelegationTargetSelectors: []core.CapabilitySelector{{
					CoordinationRoles:     []core.CoordinationRole{core.CoordinationRoleReviewer},
					CoordinationTaskTypes: []string{"review"},
				}},
			},
			ProviderPolicies: map[string]core.ProviderPolicy{
				"remote-mcp": {Activate: core.AgentPermissionAllow},
			},
			Providers: []core.ProviderConfig{{
				ID:             "remote-mcp",
				Kind:           core.ProviderKindMCPClient,
				Enabled:        true,
				Target:         "stdio://fixture",
				Recoverability: core.RecoverabilityPersistedRestore,
				Config: map[string]any{
					"command": "fixture-mcp",
					"coordination_tools": map[string]any{
						"remote.echo": map[string]any{
							"role":       "reviewer",
							"task_types": []any{"review"},
						},
					},
				},
			}},
		},
	}
	require.NoError(t, RegisterBuiltinProviders(context.Background(), rt))

	snapshot, err := rt.ExecuteDelegation(context.Background(), core.DelegationRequest{
		ID:                 "delegation-rt-remote-1",
		TaskType:           "review",
		Instruction:        "Review via MCP",
		TargetCapabilityID: "mcp:remote-mcp:tool:remote.echo",
	}, fauthorization.DelegationExecutionOptions{})
	require.NoError(t, err)
	require.Equal(t, core.DelegationStateSucceeded, snapshot.State)
	require.Equal(t, "remote-mcp", snapshot.Request.TargetProviderID)
	require.NotEmpty(t, snapshot.Request.TargetSessionID)
	require.Equal(t, core.InsertionActionMetadataOnly, snapshot.Result.Insertion.Action)
	require.Equal(t, core.TrustClassRemoteDeclared, snapshot.Result.Provenance.TrustClass)
	_ = server
}

func TestRuntimeDelegationObserverEmitsTelemetryAndAudit(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.RegisterInvocableCapability(runtimeDelegationCapability{
		desc: core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
			ID:            "relurpic:planner.plan",
			Name:          "planner.plan",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
			Coordination: &core.CoordinationTargetMetadata{
				Target:         true,
				Role:           core.CoordinationRolePlanner,
				TaskTypes:      []string{"plan"},
				ExecutionModes: []core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
			},
			InputSchema: &core.Schema{
				Type: "object",
				Properties: map[string]*core.Schema{
					"instruction": {Type: "string"},
				},
				Required: []string{"instruction"},
			},
			OutputSchema: &core.Schema{
				Type: "object",
				Properties: map[string]*core.Schema{
					"summary": {Type: "string"},
				},
				Required: []string{"summary"},
			},
		}),
	}))
	telemetry := &delegationRecordingTelemetry{}
	audit := core.NewInMemoryAuditLogger(10)
	manager := fauthorization.NewDelegationManager()
	rt := &Runtime{
		Tools:       registry,
		Context:     core.NewContext(),
		Delegations: manager,
		Telemetry:   telemetry,
		Registration: &fauthorization.AgentRegistration{
			ID:       "coding",
			Audit:    audit,
			Manifest: &manifest.AgentManifest{},
		},
		AgentSpec: &core.AgentRuntimeSpec{
			Mode:  core.AgentModePrimary,
			Model: core.AgentModelConfig{Name: "stub", Provider: "test"},
			Coordination: core.AgentCoordinationSpec{
				Enabled: true,
				DelegationTargetSelectors: []core.CapabilitySelector{{
					CoordinationRoles:     []core.CoordinationRole{core.CoordinationRolePlanner},
					CoordinationTaskTypes: []string{"plan"},
				}},
			},
		},
	}
	manager.SetObserver(rt.observeDelegationSnapshot)

	_, err := rt.ExecuteDelegation(context.Background(), core.DelegationRequest{
		ID:          "delegation-audit-1",
		TaskID:      "task-1",
		TaskType:    "plan",
		Instruction: "Plan with telemetry",
	}, fauthorization.DelegationExecutionOptions{})
	require.NoError(t, err)

	require.Len(t, telemetry.events, 2)
	require.Equal(t, core.EventDelegationStart, telemetry.events[0].Type)
	require.Equal(t, core.EventDelegationFinish, telemetry.events[1].Type)
	records, err := audit.Query(context.Background(), core.AuditQuery{Action: "delegation"})
	require.NoError(t, err)
	require.Len(t, records, 2)
	require.Equal(t, "running", records[0].Type)
	require.Equal(t, "success", records[1].Result)
}

type runtimeDelegationCapability struct {
	desc  core.CapabilityDescriptor
	delay time.Duration
}

func (c runtimeDelegationCapability) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return c.desc
}

func (c runtimeDelegationCapability) Invoke(_ context.Context, state *core.Context, args map[string]any) (*core.ToolResult, error) {
	if c.delay > 0 {
		time.Sleep(c.delay)
	}
	active, _ := state.Get("runtime.active")
	return &core.ToolResult{
		Success: true,
		Data: map[string]any{
			"summary":     "planned via runtime",
			"state_seen":  active,
			"instruction": args["instruction"],
		},
	}, nil
}
