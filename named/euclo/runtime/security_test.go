package runtime

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	testtool "github.com/lexcodex/relurpify/named/euclo/internal/testutil"
)

type securityProviderStub struct {
	desc core.ProviderDescriptor
}

func (p securityProviderStub) Descriptor() core.ProviderDescriptor { return p.desc }
func (p securityProviderStub) Initialize(context.Context, core.ProviderRuntime) error {
	return nil
}
func (p securityProviderStub) RegisterCapabilities(context.Context, core.CapabilityRegistrar) error {
	return nil
}
func (p securityProviderStub) ListSessions(context.Context) ([]core.ProviderSession, error) {
	return nil, nil
}
func (p securityProviderStub) HealthSnapshot(context.Context) (core.ProviderHealthSnapshot, error) {
	return core.ProviderHealthSnapshot{}, nil
}
func (p securityProviderStub) Close(context.Context) error { return nil }

func TestBuildSecurityRuntimeStateDiagnosesCapabilityOutsideAllowlist(t *testing.T) {
	registry := capability.NewRegistry()
	registry.UseAgentSpec("euclo", &core.AgentRuntimeSpec{
		AllowedCapabilities: []core.CapabilitySelector{{ID: "file_read", Kind: core.CapabilityKindTool}},
	})
	if err := registry.RegisterCapability(core.CapabilityDescriptor{
		ID:   "verify.go_test",
		Name: "verify.go_test",
		Kind: core.CapabilityKindTool,
	}); err != nil {
		t.Fatalf("register capability: %v", err)
	}
	cfg := &core.Config{
		AgentSpec: &core.AgentRuntimeSpec{
			AllowedCapabilities: []core.CapabilitySelector{{ID: "file_read", Kind: core.CapabilityKindTool}},
		},
	}
	security := BuildSecurityRuntimeState(cfg, registry, nil, core.NewContext(), UnitOfWork{
		ModeID: "debug",
		ExecutorDescriptor: WorkUnitExecutorDescriptor{
			Family: ExecutorFamilyHTN,
		},
		CapabilityBindings: []UnitOfWorkCapabilityBinding{{
			CapabilityID: "verify.go_test",
			Family:       "verify",
			Required:     true,
		}},
	})
	if security.Blocked {
		t.Fatalf("expected diagnostic-only security state: %#v", security)
	}
	if len(security.DeniedCapabilityUsage) != 1 || security.DeniedCapabilityUsage[0] != "verify.go_test" {
		t.Fatalf("unexpected denied capability usage: %#v", security.DeniedCapabilityUsage)
	}
	if security.ExecutionCatalogSnapshotID == "" || security.PolicySnapshotID == "" {
		t.Fatalf("expected framework snapshot metadata: %#v", security)
	}
	if len(security.AdmittedCallableCaps) != 0 {
		t.Fatalf("expected denied capability to be absent from admitted callable set: %#v", security.AdmittedCallableCaps)
	}
	if len(security.Diagnostics) == 0 || security.Diagnostics[0].Kind != "framework_catalog_mismatch" {
		t.Fatalf("expected framework catalog mismatch diagnostic: %#v", security.Diagnostics)
	}
}

func TestBuildSecurityRuntimeStateDiagnosesDeniedRequiredToolWhenPolicyConfigured(t *testing.T) {
	registry := capability.NewRegistry()
	registry.UseAgentSpec("euclo", &core.AgentRuntimeSpec{
		AllowedCapabilities: []core.CapabilitySelector{{Name: "file_write", Kind: core.CapabilityKindTool}},
	})
	if err := registry.Register(testtool.FileWriteTool{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	cfg := &core.Config{
		AgentSpec: &core.AgentRuntimeSpec{
			AllowedCapabilities: []core.CapabilitySelector{{Kind: core.CapabilityKindTool}},
		},
	}
	security := BuildSecurityRuntimeState(cfg, registry, nil, core.NewContext(), UnitOfWork{
		ModeID:               "debug",
		VerificationPolicyID: "verify.required",
		ResolvedPolicy: ResolvedExecutionPolicy{
			ProfileID:               "reproduce_localize_patch",
			RequireVerificationStep: true,
		},
		ExecutorDescriptor: WorkUnitExecutorDescriptor{Family: ExecutorFamilyHTN},
		ToolBindings: []UnitOfWorkToolBinding{
			{ToolID: "verification", Allowed: false},
		},
	})
	if security.Blocked {
		t.Fatalf("expected diagnostic-only security state: %#v", security)
	}
	if len(security.DeniedToolUsage) != 1 || security.DeniedToolUsage[0] != "verification" {
		t.Fatalf("unexpected denied tool usage: %#v", security.DeniedToolUsage)
	}
	if len(security.AdmittedModelTools) == 0 || security.AdmittedModelTools[0] != "file_write" {
		t.Fatalf("expected admitted model tools from framework catalog: %#v", security.AdmittedModelTools)
	}
}

func TestBuildSecurityRuntimeStateFlagsProviderTrustAndRecoverabilityDiagnostics(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.provider_restore", ProviderRestoreState{
		MateriallyRequired: true,
		Outcomes: []ProviderRestoreOutcome{{
			ProviderID: "provider-1",
			Reason:     "provider_restore_unsupported",
		}},
	})
	security := BuildSecurityRuntimeState(&core.Config{}, capability.NewRegistry(), []core.Provider{
		securityProviderStub{desc: core.ProviderDescriptor{
			ID:                 "provider-1",
			TrustBaseline:      core.TrustClassRemoteDeclared,
			RecoverabilityMode: core.RecoverabilityPersistedRestore,
		}},
	}, state, UnitOfWork{
		ModeID:             "planning",
		ExecutorDescriptor: WorkUnitExecutorDescriptor{Family: ExecutorFamilyRewoo},
	})
	if len(security.Diagnostics) < 2 {
		t.Fatalf("expected provider diagnostics: %#v", security.Diagnostics)
	}
}

func TestBuildSharedContextRuntimeStateSummarizesParticipantsAndMutations(t *testing.T) {
	shared := core.NewSharedContext(core.NewContext(), core.NewContextBudget(2048), &core.SimpleSummarizer{})
	shared.RecordMutation("file:pkg/service.go", "update", "euclo.executor", nil)
	rt := BuildSharedContextRuntimeState(shared, UnitOfWork{
		BehaviorFamily: "gap_analysis",
		ExecutorDescriptor: WorkUnitExecutorDescriptor{
			Family: ExecutorFamilyRewoo,
		},
		RoutineBindings: []UnitOfWorkRoutineBinding{{Family: "coherence_assessment"}},
		SkillBindings:   []UnitOfWorkSkillBinding{{SkillID: "agent_skill_policy"}},
	})
	if !rt.Enabled {
		t.Fatalf("expected shared context runtime to be enabled: %#v", rt)
	}
	if len(rt.Participants) == 0 {
		t.Fatalf("expected participants: %#v", rt)
	}
	if len(rt.RecentMutationKeys) == 0 {
		t.Fatalf("expected recent mutations: %#v", rt)
	}
}
