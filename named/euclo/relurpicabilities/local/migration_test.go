package local

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

func TestMigrationExecuteEligibleRequiresWriteAndVerificationTools(t *testing.T) {
	env := testutil.Env(t)
	cap := NewMigrationExecuteCapability(env)
	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{{
		Kind:    euclotypes.ArtifactKindIntake,
		Payload: map[string]any{"instruction": "database migration for users table"},
	}})
	snapReadOnly := eucloruntime.SnapshotCapabilities(testutil.RegistryWith(
		testutil.EchoTool{ToolName: "test_runner"},
	))
	if cap.Eligible(artifacts, snapReadOnly).Eligible {
		t.Fatal("expected ineligible without write tools")
	}
	snapOk := eucloruntime.SnapshotCapabilities(testutil.RegistryWith(
		testutil.FileWriteTool{},
		testutil.EchoTool{ToolName: "test_runner"},
	))
	if !cap.Eligible(artifacts, snapOk).Eligible {
		t.Fatal("expected eligible with write and verification-class tool names")
	}
}

func TestMigrationExecuteUsesSingleStepPlanFromState(t *testing.T) {
	env := testutil.Env(t)
	env.Registry = testutil.RegistryWith(
		testutil.FileWriteTool{},
		testutil.EchoTool{ToolName: "test_runner"},
	)
	state := core.NewContext()
	state.Set("euclo.artifacts", []euclotypes.Artifact{{
		Kind:    euclotypes.ArtifactKindIntake,
		Payload: map[string]any{"instruction": "API migration for v2 handlers"},
	}})
	state.Set("euclo.migration_plan", map[string]any{
		"migration_type": "api_upgrade",
		"steps": []map[string]any{{
			"id":             "single",
			"description":    "apply single migration step",
			"preconditions":  []string{},
			"postconditions": []string{},
			"files":          []string{},
		}},
		"completed_steps": 0,
	})
	result := NewMigrationExecuteCapability(env).Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "mig-1",
			Instruction: "API migration for v2 handlers",
			Context:     map[string]any{"workspace": t.TempDir()},
		},
		State:       state,
		Environment: env,
	})
	if result.Status != euclotypes.ExecutionStatusCompleted {
		t.Fatalf("expected completed migration, got %+v", result)
	}
	if len(result.Artifacts) < 2 {
		t.Fatalf("expected migration plan + verification artifacts, got %d", len(result.Artifacts))
	}
	final, ok := state.Get("euclo.migration_plan")
	if !ok {
		t.Fatal("expected migration plan merged into state")
	}
	plan, ok := final.(map[string]any)
	if !ok || plan["completed_steps"] != 1 {
		t.Fatalf("expected one completed step, got %#v", final)
	}
}
