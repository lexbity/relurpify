package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

func TestBuildContextRuntimeSelectsAggressiveForDebug(t *testing.T) {
	rt := BuildContextRuntime(&core.Task{Instruction: "fix the failing test"}, ContextRuntimeConfig{
		Model: testutil.StubModel{},
	}, ModeResolution{ModeID: "debug"}, UnitOfWork{
		ModeID: "debug",
		ExecutorDescriptor: WorkUnitExecutorDescriptor{
			Family: ExecutorFamilyHTN,
		},
	})
	if rt == nil {
		t.Fatal("expected context runtime")
	}
	if rt.State.StrategyName != "aggressive" {
		t.Fatalf("expected aggressive strategy, got %#v", rt.State)
	}
}

func TestBuildContextRuntimeSelectsConservativeForLongRunningPlan(t *testing.T) {
	rt := BuildContextRuntime(&core.Task{Instruction: "execute migration plan"}, ContextRuntimeConfig{
		Model: testutil.StubModel{},
	}, ModeResolution{ModeID: "planning"}, UnitOfWork{
		ModeID: "planning",
		ExecutorDescriptor: WorkUnitExecutorDescriptor{
			Family: ExecutorFamilyRewoo,
		},
		PlanBinding: &UnitOfWorkPlanBinding{
			IsPlanBacked:  true,
			IsLongRunning: true,
		},
		ContextBundle: UnitOfWorkContextBundle{
			CompactionEligible: true,
			RestoreRequired:    true,
		},
	})
	if rt == nil {
		t.Fatal("expected context runtime")
	}
	if rt.State.StrategyName != "conservative" {
		t.Fatalf("expected conservative strategy, got %#v", rt.State)
	}
	if !rt.State.CompactionEligible || !rt.State.RestoreRequired {
		t.Fatalf("expected long-running context flags, got %#v", rt.State)
	}
}

func TestContextRuntimeActivatePublishesBudgetAndInitialLoadState(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "sample.txt")
	if err := os.WriteFile(path, []byte("sample context file"), 0o644); err != nil {
		t.Fatalf("write context file: %v", err)
	}
	task := &core.Task{
		Instruction: "review sample",
		Context: map[string]any{
			"workspace":     workspace,
			"context_files": []string{path},
		},
	}
	state := core.NewContext()
	state.AddInteraction("assistant", "latest reply", nil)

	rt := BuildContextRuntime(task, ContextRuntimeConfig{
		Model: testutil.StubModel{},
	}, ModeResolution{ModeID: "review"}, UnitOfWork{
		ModeID: "review",
		ExecutorDescriptor: WorkUnitExecutorDescriptor{
			Family: ExecutorFamilyReflection,
		},
		ResolvedPolicy: ResolvedExecutionPolicy{
			ContextPolicy: ContextPolicySummary{
				MaxTokens:       9000,
				PreferredDetail: "concise",
			},
		},
	})
	if rt == nil {
		t.Fatal("expected context runtime")
	}
	got := rt.Activate(task, state, testutil.StubModel{})
	if !got.InitialLoadAttempted || !got.InitialLoadCompleted {
		t.Fatalf("expected initial load success, got %#v", got)
	}
	if got.BudgetMaxTokens == 0 || got.BudgetState == "" {
		t.Fatalf("expected budget state, got %#v", got)
	}
	raw, ok := state.Get("euclo.context_runtime")
	if !ok {
		t.Fatal("expected context runtime in state")
	}
	published, ok := raw.(ContextRuntimeState)
	if !ok {
		t.Fatalf("unexpected published type %T", raw)
	}
	if published.ExecutorFamily != ExecutorFamilyReflection {
		t.Fatalf("unexpected published runtime %#v", published)
	}
}
