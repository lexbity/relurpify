package policy

import (
	"context"
	"reflect"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	runtimepkg "github.com/lexcodex/relurpify/named/euclo/runtime"
	euclorestore "github.com/lexcodex/relurpify/named/euclo/runtime/restore"
)

type stubProvider struct {
	desc core.ProviderDescriptor
}

func (s stubProvider) Descriptor() core.ProviderDescriptor                    { return s.desc }
func (s stubProvider) Initialize(context.Context, core.ProviderRuntime) error { return nil }
func (s stubProvider) RegisterCapabilities(context.Context, core.CapabilityRegistrar) error {
	return nil
}
func (s stubProvider) ListSessions(context.Context) ([]core.ProviderSession, error) {
	return nil, nil
}
func (s stubProvider) HealthSnapshot(context.Context) (core.ProviderHealthSnapshot, error) {
	return core.ProviderHealthSnapshot{}, nil
}
func (s stubProvider) Close(context.Context) error { return nil }

func TestBuildSecurityRuntimeStateFlagsProviderTrustAndRestoreMismatch(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.provider_restore", euclorestore.ProviderRestoreState{
		MateriallyRequired: true,
		Outcomes: []euclorestore.ProviderRestoreOutcome{{
			ProviderID: "provider.test",
			Reason:     "provider_restore_failed",
		}},
	})

	security := BuildSecurityRuntimeState(nil, nil, []core.Provider{
		stubProvider{desc: core.ProviderDescriptor{
			ID:                 "provider.test",
			Kind:               core.ProviderKindPlugin,
			TrustBaseline:      core.TrustClassProviderLocalUntrusted,
			RecoverabilityMode: core.RecoverabilityPersistedRestore,
		}},
	}, state, runtimepkg.UnitOfWork{
		ModeID: "debug",
		ExecutorDescriptor: runtimepkg.WorkUnitExecutorDescriptor{
			Family: runtimepkg.ExecutorFamilyReact,
		},
	})

	if len(security.Diagnostics) < 2 {
		t.Fatalf("expected trust and restore diagnostics, got %+v", security.Diagnostics)
	}
	if len(security.DeniedCapabilityUsage) != 0 {
		t.Fatalf("unexpected denied capabilities: %+v", security.DeniedCapabilityUsage)
	}
}

func TestBuildSharedContextRuntimeStateCollectsParticipantsAndMutations(t *testing.T) {
	shared := core.NewSharedContext(core.NewContext(), nil, nil)
	if _, err := shared.AddFile("main.go", "package main", "go", core.DetailFull); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	shared.RecordMutation("pipeline.plan", "set", "agent", nil)

	rt := BuildSharedContextRuntimeState(shared, runtimepkg.UnitOfWork{
		BehaviorFamily: "debugging",
		ExecutorDescriptor: runtimepkg.WorkUnitExecutorDescriptor{
			Family: runtimepkg.ExecutorFamilyPlanner,
		},
		RoutineBindings: []runtimepkg.UnitOfWorkRoutineBinding{{Family: "review"}},
		SkillBindings:   []runtimepkg.UnitOfWorkSkillBinding{{SkillID: "skill.test"}},
	})

	if !rt.Enabled {
		t.Fatal("expected shared context runtime to be enabled")
	}
	if len(rt.WorkingSetRefs) != 1 || rt.WorkingSetRefs[0] != "main.go" {
		t.Fatalf("unexpected working set refs: %+v", rt.WorkingSetRefs)
	}
	if rt.RecentMutationCount != 1 || len(rt.RecentMutationKeys) != 1 || rt.RecentMutationKeys[0] != "pipeline.plan" {
		t.Fatalf("unexpected mutation state: %+v", rt)
	}
	if len(rt.Participants) < 4 {
		t.Fatalf("expected executor, behavior, routine, and skill participants: %+v", rt.Participants)
	}
}

func TestBuildPolicyDoesNotMutateTask(t *testing.T) {
	task := &core.Task{
		ID:          "task-1",
		Instruction: "inspect the code",
		Context:     map[string]any{"mode": "chat"},
	}
	before := reflect.ValueOf(*task).Interface()

	_ = BuildPolicy(task, &core.Config{}, nil, runtimepkg.ModeResolution{ModeID: "chat"}, runtimepkg.ExecutionProfileSelection{ProfileID: "review_suggest_implement"})

	after := reflect.ValueOf(*task).Interface()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("expected task to remain unchanged, before=%#v after=%#v", before, after)
	}
}
