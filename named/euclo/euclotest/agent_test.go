package euclotest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/named/euclo"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/lexcodex/relurpify/named/euclo/internal/testutil"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	"github.com/stretchr/testify/require"
)

func TestAgentExecutePublishesNormalizedArtifacts(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})

	state := core.NewContext()
	state.Set("pipeline.workflow_retrieval", map[string]any{"summary": "prior context"})
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "task-1",
		Instruction: "summarize current status",
		Context:     map[string]any{"workspace": "/tmp/ws"},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, "code", state.GetString("euclo.mode"))
	require.Equal(t, "plan_stage_execute", state.GetString("euclo.execution_profile"))

	classificationRaw, ok := state.Get("euclo.classification")
	require.True(t, ok)
	classification, ok := classificationRaw.(eucloruntime.TaskClassification)
	require.True(t, ok)
	require.Equal(t, "code", classification.RecommendedMode)

	raw, ok := state.Get("euclo.artifacts")
	require.True(t, ok)
	artifacts, ok := raw.([]euclotypes.Artifact)
	require.True(t, ok)
	require.NotEmpty(t, artifacts)
	require.Equal(t, euclotypes.ArtifactKindIntake, artifacts[0].Kind)
}

func TestAgentExecuteAppliesPendingEditIntentsThroughRegistry(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})

	target := filepath.Join(t.TempDir(), "note.txt")
	state := core.NewContext()
	state.Set("pipeline.code", map[string]any{
		"summary": "one write",
		"edits": []any{
			map[string]any{"path": target, "action": "update", "content": "done", "summary": "write file"},
		},
	})
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})

	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-2",
		Instruction: "implement the requested change",
		Context:     map[string]any{"workspace": "/tmp/ws"},
	}, state)
	require.NoError(t, err)

	raw, ok := state.Get("euclo.edit_execution")
	require.True(t, ok)
	record, ok := raw.(eucloruntime.EditExecutionRecord)
	require.True(t, ok)
	require.Len(t, record.Executed, 1)

	data, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	require.Equal(t, "done", string(data))
}

func TestAgentExecuteFailsWhenVerificationIsMissingForMutatingProfile(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})

	target := filepath.Join(t.TempDir(), "note.txt")
	state := core.NewContext()
	state.Set("pipeline.code", map[string]any{
		"summary": "one write",
		"edits": []any{
			map[string]any{"path": target, "action": "update", "content": "done", "summary": "write file"},
		},
	})

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "task-3",
		Instruction: "implement the requested change",
		Context:     map[string]any{"workspace": "/tmp/ws"},
	}, state)
	require.Error(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Contains(t, err.Error(), "success gate blocked")

	raw, ok := state.Get("euclo.success_gate")
	require.True(t, ok)
	gate, ok := raw.(eucloruntime.SuccessGateResult)
	require.True(t, ok)
	require.False(t, gate.Allowed)
	require.Equal(t, "verification_missing", gate.Reason)
}

func TestAgentExecuteSeedsDebugInteractionSkipState(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-skip-debug",
		Instruction: "panic: runtime error: nil pointer dereference at server.go:42",
		Context:     map[string]any{"workspace": "/tmp/ws"},
	}, state)
	require.NoError(t, err)

	raw, ok := state.Get("euclo.interaction_state")
	require.True(t, ok)
	iState, ok := raw.(interaction.InteractionState)
	require.True(t, ok)
	require.Equal(t, "debug", iState.Mode)
	require.Contains(t, iState.SkippedPhases, "intake")
}

func TestAgentExecuteSeedsCodeFastPathSkipState(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-skip-code",
		Instruction: "just do it and rename the function foo to bar in util.go",
		Context:     map[string]any{"workspace": "/tmp/ws"},
	}, state)
	require.NoError(t, err)

	raw, ok := state.Get("euclo.interaction_state")
	require.True(t, ok)
	iState, ok := raw.(interaction.InteractionState)
	require.True(t, ok)
	require.Equal(t, "code", iState.Mode)
	require.Contains(t, iState.SkippedPhases, "propose")
}

func TestAgentExecuteSeedsPlanningFastPathSkipState(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})

	state := core.NewContext()
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-skip-planning",
		Instruction: "just plan it: add authentication to the API",
		Context:     map[string]any{"workspace": "/tmp/ws", "mode": "planning"},
	}, state)
	require.NoError(t, err)

	raw, ok := state.Get("euclo.interaction_state")
	require.True(t, ok)
	iState, ok := raw.(interaction.InteractionState)
	require.True(t, ok)
	require.Equal(t, "planning", iState.Mode)
	require.Contains(t, iState.SkippedPhases, "clarify")
	require.Contains(t, iState.SkippedPhases, "compare")
	require.Contains(t, iState.SkippedPhases, "refine")
}

func TestAgentExecuteScriptedTransitionRejectStaysInCode(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-transition-reject",
		Instruction: "add logging to all API handlers",
		Context: map[string]any{
			"workspace": "/tmp/ws",
			"euclo.interaction_script": []map[string]any{
				{"phase": "understand", "action": "plan_first"},
				{"kind": "transition", "action": "reject"},
			},
		},
	}, state)
	require.NoError(t, err)

	raw, ok := state.Get("euclo.interaction_state")
	require.True(t, ok)
	iState, ok := raw.(interaction.InteractionState)
	require.True(t, ok)
	require.Equal(t, "code", iState.Mode)
	require.Contains(t, iState.PhasesExecuted, "execute")

	recordingRaw, ok := state.Get("euclo.interaction_recording")
	require.True(t, ok)
	recording, ok := recordingRaw.(map[string]any)
	require.True(t, ok)
	transitions, ok := recording["transitions"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, transitions, 1)
}

func TestAgentExecuteScriptedRoundTripCodePlanningCode(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-transition-roundtrip",
		Instruction: "add logging to all API handlers",
		Context: map[string]any{
			"workspace": "/tmp/ws",
			"euclo.interaction_script": []map[string]any{
				{"phase": "understand", "action": "plan_first"},
				{"kind": "transition", "action": "accept"},
				{"kind": "transition", "action": "accept"},
			},
		},
	}, state)
	require.NoError(t, err)

	raw, ok := state.Get("euclo.interaction_state")
	require.True(t, ok)
	iState, ok := raw.(interaction.InteractionState)
	require.True(t, ok)
	require.Equal(t, "code", iState.Mode)
	require.Contains(t, iState.PhasesExecuted, "scope")
	require.Contains(t, iState.PhasesExecuted, "generate")
	require.Contains(t, iState.PhasesExecuted, "commit")
	require.Contains(t, iState.PhasesExecuted, "execute")

	recordingRaw, ok := state.Get("euclo.interaction_recording")
	require.True(t, ok)
	recording, ok := recordingRaw.(map[string]any)
	require.True(t, ok)
	transitions, ok := recording["transitions"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, transitions, 2)
}

func TestAgentExecuteSeedsPersistedInteractionStateFromTaskContext(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})

	state := core.NewContext()
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-resume-seed",
		Instruction: "plan and implement rate limiting",
		Context: map[string]any{
			"workspace":  "/tmp/ws",
			"euclo.mode": "planning",
			"euclo.interaction_state": map[string]any{
				"mode":            "planning",
				"current_phase":   "generate",
				"phases_executed": []any{"scope", "clarify"},
				"phase_states": map[string]any{
					"scope.done":   true,
					"clarify.done": true,
				},
			},
			"euclo.interaction_script": []map[string]any{
				{"kind": "session_resume", "action": "resume"},
			},
		},
	}, state)
	require.NoError(t, err)

	raw, ok := state.Get("euclo.interaction_state")
	require.True(t, ok)
	iState, ok := raw.(interaction.InteractionState)
	require.True(t, ok)
	require.Contains(t, iState.PhasesExecuted, "generate")

	recordingRaw, ok := state.Get("euclo.interaction_recording")
	require.True(t, ok)
	recording, ok := recordingRaw.(map[string]any)
	require.True(t, ok)
	frames, ok := recording["frames"].([]map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, frames)
	require.Equal(t, "session_resume", frames[0]["kind"])
}
