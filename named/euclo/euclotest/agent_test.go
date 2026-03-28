package euclotest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graphdb"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/framework/retrieval"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/lexcodex/relurpify/named/euclo"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	clinix "github.com/lexcodex/relurpify/platform/shell/command"
	"github.com/lexcodex/relurpify/testutil/euclotestutil"
	"github.com/stretchr/testify/require"
)

type memoryPlanStore struct {
	plans               map[string]*frameworkplan.LivingPlan
	updates             map[string]*frameworkplan.PlanStep
	loadByWorkflowCount int
}

func newMemoryPlanStore() *memoryPlanStore {
	return &memoryPlanStore{
		plans:   make(map[string]*frameworkplan.LivingPlan),
		updates: make(map[string]*frameworkplan.PlanStep),
	}
}

func registerCliGitForRepo(t *testing.T, registry *capability.Registry, repo string) {
	t.Helper()
	tool := clinix.NewCommandTool(repo, clinix.CommandToolConfig{
		Name:        "cli_git",
		Description: "Runs git with the provided arguments.",
		Command:     "git",
		Category:    "git",
		Tags:        []string{core.TagExecute},
	})
	tool.SetCommandRunner(fsandbox.NewLocalCommandRunner(repo, nil))
	require.NoError(t, registry.Register(tool))
}

func (s *memoryPlanStore) SavePlan(_ context.Context, plan *frameworkplan.LivingPlan) error {
	s.plans[plan.WorkflowID] = plan
	return nil
}

func (s *memoryPlanStore) LoadPlan(_ context.Context, planID string) (*frameworkplan.LivingPlan, error) {
	for _, plan := range s.plans {
		if plan.ID == planID {
			return plan, nil
		}
	}
	return nil, nil
}

func (s *memoryPlanStore) LoadPlanByWorkflow(_ context.Context, workflowID string) (*frameworkplan.LivingPlan, error) {
	s.loadByWorkflowCount++
	return s.plans[workflowID], nil
}

func (s *memoryPlanStore) UpdateStep(_ context.Context, planID, stepID string, step *frameworkplan.PlanStep) error {
	stepCopy := *step
	s.updates[planID+":"+stepID] = &stepCopy
	for _, plan := range s.plans {
		if plan != nil && plan.ID == planID {
			plan.Steps[stepID] = &stepCopy
			break
		}
	}
	return nil
}

func (s *memoryPlanStore) InvalidateStep(_ context.Context, _ string, _ string, _ frameworkplan.InvalidationRule) error {
	return nil
}

func (s *memoryPlanStore) DeletePlan(_ context.Context, _ string) error { return nil }
func (s *memoryPlanStore) ListPlans(_ context.Context) ([]frameworkplan.PlanSummary, error) {
	return nil, nil
}

type stubConvergenceVerifier struct {
	called bool
	fail   *frameworkplan.ConvergenceFailure
	err    error
}

func resolveNextGuidanceRequest(t *testing.T, broker *guidance.GuidanceBroker, choiceID string) {
	t.Helper()
	events, cancel := broker.Subscribe(8)
	t.Cleanup(cancel)
	go func() {
		for event := range events {
			if event.Type != guidance.GuidanceEventRequested || event.Request == nil {
				continue
			}
			_ = broker.Resolve(guidance.GuidanceDecision{
				RequestID: event.Request.ID,
				ChoiceID:  choiceID,
			})
			return
		}
	}()
}

func (s *stubConvergenceVerifier) Verify(_ context.Context, target frameworkplan.ConvergenceTarget) (*frameworkplan.ConvergenceFailure, error) {
	s.called = true
	if s.err != nil {
		return nil, s.err
	}
	if s.fail != nil {
		return s.fail, nil
	}
	_ = target
	return nil, nil
}

func TestAgentExecutePublishesNormalizedArtifacts(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	workspace := initGitRepo(t)
	registry := capability.NewRegistry()
	registerCliGitForRepo(t, registry, workspace)
	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
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
	workspace := initGitRepo(t)
	registry := capability.NewRegistry()
	registerCliGitForRepo(t, registry, workspace)
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

func TestAgentExecuteLoadsLivingPlanIntoState(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))
	store := newMemoryPlanStore()
	now := time.Now().UTC()
	store.plans["wf-1"] = &frameworkplan.LivingPlan{
		ID:         "plan-1",
		WorkflowID: "wf-1",
		Title:      "loaded plan",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.PlanStore = store

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-plan-load",
		Instruction: "summarize status",
		Context:     map[string]any{"workspace": "/tmp/ws", "workflow_id": "wf-1"},
	}, state)
	require.NoError(t, err)

	raw, ok := state.Get("euclo.living_plan")
	require.True(t, ok)
	loaded, ok := raw.(*frameworkplan.LivingPlan)
	require.True(t, ok)
	require.Equal(t, "plan-1", loaded.ID)
}

func TestAgentExecuteSummaryFastPathSkipsPlanPreparationWhenNothingIsBlocking(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	workflowStore := openRetrievalWorkflowStore(t)
	require.NoError(t, workflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-fast",
		TaskID:      "task-fast",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "summarize current status",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	store := newMemoryPlanStore()
	now := time.Now().UTC()
	store.plans["wf-blocking"] = &frameworkplan.LivingPlan{
		ID:         "plan-blocking",
		WorkflowID: "wf-blocking",
		Title:      "blocking plan",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.WorkflowStore = workflowStore
	agent.PlanStore = store

	state := core.NewContext()
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-fast",
		Instruction: "summarize current status",
		Context: map[string]any{
			"workspace": "/tmp/ws",
		},
	}, state)
	require.NoError(t, err)
	require.Zero(t, store.loadByWorkflowCount)
}

func TestAgentExecuteSummaryFastPathDoesNotSkipBlockingLearning(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	workflowStore := openRetrievalWorkflowStore(t)
	require.NoError(t, workflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-blocking",
		TaskID:      "task-blocking",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "summarize current status",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	learnSvc := archaeolearning.Service{Store: workflowStore}
	_, err = learnSvc.Create(context.Background(), archaeolearning.CreateInput{
		WorkflowID:    "wf-blocking",
		ExplorationID: "exp-blocking",
		Kind:          archaeolearning.InteractionPatternProposal,
		SubjectType:   archaeolearning.SubjectPattern,
		SubjectID:     "pattern-blocking",
		Title:         "Confirm pattern",
		Blocking:      true,
	})
	require.NoError(t, err)

	store := newMemoryPlanStore()
	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.WorkflowStore = workflowStore
	agent.PlanStore = store

	state := core.NewContext()
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-blocking",
		Instruction: "summarize current status",
		Context: map[string]any{
			"workspace":   "/tmp/ws",
			"workflow_id": "wf-blocking",
		},
	}, state)
	require.NoError(t, err)
	require.Greater(t, store.loadByWorkflowCount, 0)
}

func TestAgentExecuteAppliesLearningResolutionThroughService(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(testutil.FileWriteTool{}))
	workflowStore := openRetrievalWorkflowStore(t)
	require.NoError(t, workflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-learning",
		TaskID:      "task-learning",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "learn",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	patternStore, commentStore := openPatternStores(t)
	require.NoError(t, patternStore.Save(context.Background(), patterns.PatternRecord{
		ID:           "pattern-1",
		Kind:         patterns.PatternKindStructural,
		Title:        "Adapter",
		Description:  "Use adapters",
		Status:       patterns.PatternStatusProposed,
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}))
	learnSvc := archaeolearning.Service{
		Store:        workflowStore,
		PatternStore: patternStore,
		CommentStore: commentStore,
	}
	interaction, err := learnSvc.Create(context.Background(), archaeolearning.CreateInput{
		WorkflowID:    "wf-learning",
		ExplorationID: "explore-1",
		Kind:          archaeolearning.InteractionPatternProposal,
		SubjectType:   archaeolearning.SubjectPattern,
		SubjectID:     "pattern-1",
		Title:         "Confirm pattern",
	})
	require.NoError(t, err)

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.WorkflowStore = workflowStore
	agent.PatternStore = patternStore
	agent.CommentStore = commentStore

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-learning-resolve",
		Instruction: "summarize status",
		Context: map[string]any{
			"workspace":    "/tmp/ws",
			"workflow_id":  "wf-learning",
			"corpus_scope": "workspace",
			"euclo.learning_resolution": map[string]any{
				"interaction_id":  interaction.ID,
				"resolution_kind": "confirm",
				"choice_id":       "confirm",
				"resolved_by":     "human",
				"comment": map[string]any{
					"intent_type": "intentional",
					"author_kind": "human",
					"body":        "Confirmed during archaeology review.",
				},
			},
		},
	}, state)
	require.NoError(t, err)

	record, err := patternStore.Load(context.Background(), "pattern-1")
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, patterns.PatternStatusConfirmed, record.Status)

	raw, ok := state.Get("euclo.last_learning_resolution")
	require.True(t, ok)
	resolved, ok := raw.(*archaeolearning.Interaction)
	require.True(t, ok)
	require.Equal(t, archaeolearning.StatusResolved, resolved.Status)

	raw, ok = state.Get("euclo.learning_queue")
	require.True(t, ok)
	queue, ok := raw.([]archaeolearning.Interaction)
	require.True(t, ok)
	require.Empty(t, queue)
}

func TestAgentExecuteBlocksWhenRequiredSymbolsAreMissing(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	store := newMemoryPlanStore()
	now := time.Now().UTC()
	store.plans["wf-missing"] = &frameworkplan.LivingPlan{
		ID:         "plan-missing",
		WorkflowID: "wf-missing",
		Title:      "blocked plan",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {
				ID:           "step-1",
				Status:       frameworkplan.PlanStepPending,
				EvidenceGate: &frameworkplan.EvidenceGate{RequiredSymbols: []string{"missing.symbol"}},
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	graph, err := graphdb.Open(graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	defer graph.Close()

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.PlanStore = store
	agent.GraphDB = graph

	state := core.NewContext()
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-block",
		Instruction: "do the thing",
		Context: map[string]any{
			"workspace":       "/tmp/ws",
			"workflow_id":     "wf-missing",
			"current_step_id": "step-1",
		},
	}, state)
	require.Error(t, err)
	require.Contains(t, err.Error(), "blocked by missing required symbols")

	updated := store.updates["plan-missing:step-1"]
	require.NotNil(t, updated)
	require.NotEmpty(t, updated.History)
	require.Equal(t, "blocked", updated.History[len(updated.History)-1].Outcome)
}

func TestAgentExecuteUpdatesPlanStepAndRunsConvergenceVerifier(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	store := newMemoryPlanStore()
	now := time.Now().UTC()
	store.plans["wf-complete"] = &frameworkplan.LivingPlan{
		ID:         "plan-complete",
		WorkflowID: "wf-complete",
		Title:      "complete plan",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, ConfidenceScore: 0.1, CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		ConvergenceTarget: &frameworkplan.ConvergenceTarget{
			PatternIDs: []string{"pattern-1"},
			Commentary: "done means coherent",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	graph, err := graphdb.Open(graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	defer graph.Close()
	require.NoError(t, graph.UpsertNode(graphdb.NodeRecord{ID: "present.symbol", Kind: "function"}))
	verifier := &stubConvergenceVerifier{}
	workspace := initGitRepo(t)
	registry := capability.NewRegistry()
	registerCliGitForRepo(t, registry, workspace)

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.PlanStore = store
	agent.GraphDB = graph
	agent.ConvVerifier = verifier

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-complete",
		Instruction: "finish step",
		Context: map[string]any{
			"workspace":       workspace,
			"workflow_id":     "wf-complete",
			"current_step_id": "step-1",
		},
	}, state)
	require.NoError(t, err)

	updated := store.updates["plan-complete:step-1"]
	require.NotNil(t, updated)
	require.Equal(t, frameworkplan.PlanStepCompleted, updated.Status)
	require.NotEmpty(t, updated.History)
	require.Equal(t, "completed", updated.History[len(updated.History)-1].Outcome)
	require.True(t, verifier.called)
	require.NotNil(t, store.plans["wf-complete"].ConvergenceTarget.VerifiedAt)
}

func TestAgentExecutePersistsArchaeoPhaseState(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	workflowStore := openRetrievalWorkflowStore(t)
	store := newMemoryPlanStore()
	now := time.Now().UTC()
	store.plans["wf-phase"] = &frameworkplan.LivingPlan{
		ID:         "plan-phase",
		WorkflowID: "wf-phase",
		Version:    2,
		Title:      "phase plan",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	workspace := initGitRepo(t)
	registry := capability.NewRegistry()
	registerCliGitForRepo(t, registry, workspace)

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: registry,
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.PlanStore = store
	agent.WorkflowStore = workflowStore

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-phase",
		Instruction: "finish step",
		Context: map[string]any{
			"workspace":       workspace,
			"workflow_id":     "wf-phase",
			"current_step_id": "step-1",
		},
	}, state)
	require.NoError(t, err)

	raw, ok := state.Get("euclo.archaeo_phase_state")
	require.True(t, ok)
	phaseState, ok := raw.(*archaeodomain.WorkflowPhaseState)
	require.True(t, ok)
	require.Equal(t, archaeodomain.PhaseCompleted, phaseState.CurrentPhase)
	require.Equal(t, "plan-phase", phaseState.ActivePlanID)
	require.NotNil(t, phaseState.ActivePlanVersion)
	require.Equal(t, 1, *phaseState.ActivePlanVersion)
	require.Equal(t, "plan-phase:v1", state.GetString("euclo.execution_handoff_ref"))

	artifacts, err := workflowStore.ListWorkflowArtifacts(context.Background(), "wf-phase", "")
	require.NoError(t, err)
	require.NotEmpty(t, artifacts)
	foundPhaseState := false
	for _, artifact := range artifacts {
		if artifact.Kind == "archaeo_phase_state" {
			foundPhaseState = true
			break
		}
	}
	require.True(t, foundPhaseState)
}

func TestAgentExecuteMarksPlanStepFailedOnExecutionFailure(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	store := newMemoryPlanStore()
	now := time.Now().UTC()
	store.plans["wf-fail"] = &frameworkplan.LivingPlan{
		ID:         "plan-fail",
		WorkflowID: "wf-fail",
		Title:      "failing plan",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.PlanStore = store

	state := core.NewContext()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = agent.Execute(ctx, &core.Task{
		ID:          "task-fail",
		Instruction: "summarize current status",
		Context: map[string]any{
			"workspace":       "/tmp/ws",
			"workflow_id":     "wf-fail",
			"current_step_id": "step-1",
		},
	}, state)
	require.Error(t, err)

	updated := store.updates["plan-fail:step-1"]
	require.NotNil(t, updated)
	require.Equal(t, frameworkplan.PlanStepFailed, updated.Status)
	require.Equal(t, "failed", updated.History[len(updated.History)-1].Outcome)
}

func TestAgentExecuteBlocksWhenRequiredAnchorsAreInactive(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	store := newMemoryPlanStore()
	now := time.Now().UTC()
	store.plans["wf-anchor"] = &frameworkplan.LivingPlan{
		ID:         "plan-anchor",
		WorkflowID: "wf-anchor",
		Title:      "anchor gated",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {
				ID:                 "step-1",
				Status:             frameworkplan.PlanStepPending,
				AnchorDependencies: []string{"anchor-1"},
				EvidenceGate:       &frameworkplan.EvidenceGate{RequiredAnchors: []string{"anchor-1"}},
				CreatedAt:          now,
				UpdatedAt:          now,
			},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	workflowStore := openRetrievalWorkflowStore(t)
	record, err := retrieval.DeclareAnchor(context.Background(), workflowStore.DB(), retrieval.AnchorDeclaration{
		Term:       "ownership",
		Definition: "feature owner assigned",
	}, "workspace", "workspace_trusted")
	require.NoError(t, err)
	insertAnchorDriftEvent(t, workflowStore, record.AnchorID)
	store.plans["wf-anchor"].Steps["step-1"].AnchorDependencies = []string{record.AnchorID}
	store.plans["wf-anchor"].Steps["step-1"].EvidenceGate.RequiredAnchors = []string{record.AnchorID}

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.PlanStore = store
	agent.RetrievalDB = workflowStore.DB()

	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-anchor-block",
		Instruction: "do the thing",
		Context: map[string]any{
			"workspace":       "/tmp/ws",
			"workflow_id":     "wf-anchor",
			"current_step_id": "step-1",
		},
	}, core.NewContext())
	require.Error(t, err)
	require.Contains(t, err.Error(), "inactive required anchors")

	updated := store.updates["plan-anchor:step-1"]
	require.NotNil(t, updated)
	require.Equal(t, frameworkplan.PlanStepInvalidated, updated.Status)
	require.Equal(t, "blocked", updated.History[len(updated.History)-1].Outcome)
}

func TestAgentExecuteLowConfidenceProceedsWhenGuided(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	store := newMemoryPlanStore()
	now := time.Now().UTC()
	store.plans["wf-guided-proceed"] = &frameworkplan.LivingPlan{
		ID:         "plan-guided-proceed",
		WorkflowID: "wf-guided-proceed",
		Title:      "guided proceed",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, ConfidenceScore: 0.1, CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	broker := guidance.NewGuidanceBroker(time.Second)
	resolveNextGuidanceRequest(t, broker, "proceed")

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.PlanStore = store
	agent.GuidanceBroker = broker

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "task-guided-proceed",
		Instruction: "finish step",
		Context: map[string]any{
			"workspace":       "/tmp/ws",
			"workflow_id":     "wf-guided-proceed",
			"current_step_id": "step-1",
		},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)
	updated := store.updates["plan-guided-proceed:step-1"]
	require.NotNil(t, updated)
	require.Equal(t, frameworkplan.PlanStepCompleted, updated.Status)
}

func TestAgentExecuteLowConfidenceSkipSkipsStep(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	store := newMemoryPlanStore()
	now := time.Now().UTC()
	store.plans["wf-guided-skip"] = &frameworkplan.LivingPlan{
		ID:         "plan-guided-skip",
		WorkflowID: "wf-guided-skip",
		Title:      "guided skip",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, ConfidenceScore: 0.1, CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	broker := guidance.NewGuidanceBroker(time.Second)
	resolveNextGuidanceRequest(t, broker, "skip")

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.PlanStore = store
	agent.GuidanceBroker = broker

	state := core.NewContext()
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "task-guided-skip",
		Instruction: "finish step",
		Context: map[string]any{
			"workspace":       "/tmp/ws",
			"workflow_id":     "wf-guided-skip",
			"current_step_id": "step-1",
		},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Equal(t, "skipped", result.Data["plan_step_status"])
	updated := store.updates["plan-guided-skip:step-1"]
	require.NotNil(t, updated)
	require.Equal(t, frameworkplan.PlanStepSkipped, updated.Status)
}

func TestAgentExecuteLowConfidenceReplanFailsStep(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	store := newMemoryPlanStore()
	now := time.Now().UTC()
	store.plans["wf-guided-replan"] = &frameworkplan.LivingPlan{
		ID:         "plan-guided-replan",
		WorkflowID: "wf-guided-replan",
		Title:      "guided replan",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, ConfidenceScore: 0.1, CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	broker := guidance.NewGuidanceBroker(time.Second)
	resolveNextGuidanceRequest(t, broker, "replan")

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.PlanStore = store
	agent.GuidanceBroker = broker

	state := core.NewContext()
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "task-guided-replan",
		Instruction: "finish step",
		Context: map[string]any{
			"workspace":       "/tmp/ws",
			"workflow_id":     "wf-guided-replan",
			"current_step_id": "step-1",
		},
	}, state)
	require.Error(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	updated := store.updates["plan-guided-replan:step-1"]
	require.NotNil(t, updated)
	require.Equal(t, frameworkplan.PlanStepFailed, updated.Status)
}

func TestAgentExecuteSurfacesDeferredGuidancePlan(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	store := newMemoryPlanStore()
	now := time.Now().UTC()
	store.plans["wf-defer"] = &frameworkplan.LivingPlan{
		ID:         "plan-defer",
		WorkflowID: "wf-defer",
		Title:      "deferred guidance",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, ConfidenceScore: 0.1, CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.PlanStore = store
	agent.GuidanceBroker = guidance.NewGuidanceBroker(5 * time.Millisecond)
	agent.DeferralPolicy = guidance.DefaultDeferralPolicy()

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "task-defer",
		Instruction: "finish step",
		Context: map[string]any{
			"workspace":       "/tmp/ws",
			"workflow_id":     "wf-defer",
			"current_step_id": "step-1",
		},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)
	raw, ok := state.Get("euclo.deferral_plan")
	require.True(t, ok)
	dp, ok := raw.(*guidance.DeferralPlan)
	require.True(t, ok)
	require.False(t, dp.IsEmpty())
}

func TestAgentExecuteBlastRadiusGuidanceTriggers(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	store := newMemoryPlanStore()
	now := time.Now().UTC()
	store.plans["wf-impact"] = &frameworkplan.LivingPlan{
		ID:         "plan-impact",
		WorkflowID: "wf-impact",
		Title:      "impact guidance",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, ConfidenceScore: 1.0, Scope: []string{"root"}, CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	graph, err := graphdb.Open(graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	defer graph.Close()
	require.NoError(t, graph.UpsertNode(graphdb.NodeRecord{ID: "root", Kind: "function"}))
	for i := 0; i < 7; i++ {
		child := filepath.Join("child", strconv.Itoa(i))
		require.NoError(t, graph.UpsertNode(graphdb.NodeRecord{ID: child, Kind: "function"}))
		require.NoError(t, graph.Link("root", child, "calls", "", 1, nil))
	}

	broker := guidance.NewGuidanceBroker(time.Second)
	requests := make(chan guidance.GuidanceRequest, 1)
	events, cancel := broker.Subscribe(8)
	defer cancel()
	go func() {
		for event := range events {
			if event.Type != guidance.GuidanceEventRequested || event.Request == nil {
				continue
			}
			requests <- *event.Request
			_ = broker.Resolve(guidance.GuidanceDecision{RequestID: event.Request.ID, ChoiceID: "proceed"})
			return
		}
	}()

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.PlanStore = store
	agent.GraphDB = graph
	agent.GuidanceBroker = broker

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "task-impact",
		Instruction: "finish step",
		Context: map[string]any{
			"workspace":       "/tmp/ws",
			"workflow_id":     "wf-impact",
			"current_step_id": "step-1",
		},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)
	select {
	case req := <-requests:
		require.Equal(t, guidance.GuidanceScopeExpansion, req.Kind)
	case <-time.After(time.Second):
		t.Fatal("expected blast radius guidance request")
	}
}

func TestAgentExecuteResetsDoomLoopDetectorPerPlanStep(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	store := newMemoryPlanStore()
	now := time.Now().UTC()
	store.plans["wf-reset"] = &frameworkplan.LivingPlan{
		ID:         "plan-reset",
		WorkflowID: "wf-reset",
		Title:      "reset detector",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, ConfidenceScore: 1.0, CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	detector := capability.NewDoomLoopDetector(capability.DefaultDoomLoopConfig())
	desc := core.CapabilityDescriptor{ID: "tool:write"}
	args := map[string]any{"path": "a.go"}
	require.NoError(t, detector.Check(desc, args))
	require.NoError(t, detector.Record(desc, &core.ToolResult{Success: true}))
	require.NoError(t, detector.Check(desc, args))
	require.NoError(t, detector.Record(desc, &core.ToolResult{Success: true}))

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.PlanStore = store
	agent.DoomLoop = detector

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-reset",
		Instruction: "finish step",
		Context: map[string]any{
			"workspace":       "/tmp/ws",
			"workflow_id":     "wf-reset",
			"current_step_id": "step-1",
		},
	}, state)
	require.NoError(t, err)
	require.NoError(t, detector.Check(desc, args))
}

func TestAgentExecuteBlocksWhenEvidenceDerivationLossExceedsGate(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	store := newMemoryPlanStore()
	now := time.Now().UTC()
	store.plans["wf-loss"] = &frameworkplan.LivingPlan{
		ID:         "plan-loss",
		WorkflowID: "wf-loss",
		Title:      "loss gated",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {
				ID:           "step-1",
				Status:       frameworkplan.PlanStepPending,
				EvidenceGate: &frameworkplan.EvidenceGate{MaxTotalLoss: 0.2},
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
		StepOrder: []string{"step-1"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	origin := core.OriginDerivation("retrieval")
	derived := origin.Derive("compress_summarize", "contextmgr", 0.4, "lossy")
	payload := retrieval.BuildMixedEvidencePayload("query", "workflow:wf-loss", retrieval.RetrievalEvent{}, []retrieval.MixedEvidenceResult{{
		Text:       "grounding text",
		Derivation: &derived,
	}})

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.PlanStore = store

	state := core.NewContext()
	state.Set("pipeline.workflow_retrieval", payload)
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-loss-block",
		Instruction: "do the thing",
		Context: map[string]any{
			"workspace":       "/tmp/ws",
			"workflow_id":     "wf-loss",
			"current_step_id": "step-1",
		},
	}, state)
	require.Error(t, err)
	require.Contains(t, err.Error(), "evidence derivation loss")
}

func TestAgentExecuteSurfacesConvergenceFailure(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	store := newMemoryPlanStore()
	now := time.Now().UTC()
	store.plans["wf-conv-fail"] = &frameworkplan.LivingPlan{
		ID:         "plan-conv-fail",
		WorkflowID: "wf-conv-fail",
		Title:      "convergence failure",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {ID: "step-1", Status: frameworkplan.PlanStepPending, CreatedAt: now, UpdatedAt: now},
		},
		StepOrder: []string{"step-1"},
		ConvergenceTarget: &frameworkplan.ConvergenceTarget{
			PatternIDs: []string{"pattern-1"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.PlanStore = store
	agent.ConvVerifier = &stubConvergenceVerifier{
		fail: &frameworkplan.ConvergenceFailure{
			UnconfirmedPatterns: []string{"pattern-1"},
			Description:         "pattern still tentative",
		},
	}

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "task-conv-fail",
		Instruction: "finish step",
		Context: map[string]any{
			"workspace":       "/tmp/ws",
			"workflow_id":     "wf-conv-fail",
			"current_step_id": "step-1",
		},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, result.Data, "convergence_failure")
	_, ok := state.Get("euclo.convergence_failure")
	require.True(t, ok)
}

func TestAgentExecuteInvalidatesDependentStepsWhenScopedSymbolChanges(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	store := newMemoryPlanStore()
	now := time.Now().UTC()
	store.plans["wf-invalidate"] = &frameworkplan.LivingPlan{
		ID:         "plan-invalidate",
		WorkflowID: "wf-invalidate",
		Title:      "invalidate dependents",
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {
				ID:        "step-1",
				Status:    frameworkplan.PlanStepPending,
				Scope:     []string{"present.symbol"},
				CreatedAt: now,
				UpdatedAt: now,
			},
			"step-2": {
				ID:        "step-2",
				Status:    frameworkplan.PlanStepPending,
				DependsOn: []string{"step-1"},
				InvalidatedBy: []frameworkplan.InvalidationRule{{
					Kind:   frameworkplan.InvalidationSymbolChanged,
					Target: "present.symbol",
				}},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		StepOrder: []string{"step-1", "step-2"},
		CreatedAt: now,
		UpdatedAt: now,
	}
	graph, err := graphdb.Open(graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	defer graph.Close()
	require.NoError(t, graph.UpsertNode(graphdb.NodeRecord{ID: "present.symbol", Kind: "function"}))

	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-test", Model: "stub", MaxIterations: 1},
	})
	agent.PlanStore = store
	agent.GraphDB = graph

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-invalidate",
		Instruction: "finish step",
		Context: map[string]any{
			"workspace":       "/tmp/ws",
			"workflow_id":     "wf-invalidate",
			"current_step_id": "step-1",
		},
	}, state)
	require.NoError(t, err)

	updated := store.updates["plan-invalidate:step-2"]
	require.NotNil(t, updated)
	require.Equal(t, frameworkplan.PlanStepInvalidated, updated.Status)
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644))
	run("add", "README.md")
	run("commit", "-m", "init")
	return dir
}

func openRetrievalWorkflowStore(t *testing.T) *memorydb.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	require.NoError(t, store.EnsureRetrievalSchema(context.Background()))
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})
	return store
}

func openPatternStores(t *testing.T) (*patterns.SQLitePatternStore, *patterns.SQLiteCommentStore) {
	t.Helper()
	db, err := patterns.OpenSQLite(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	patternStore, err := patterns.NewSQLitePatternStore(db)
	require.NoError(t, err)
	commentStore, err := patterns.NewSQLiteCommentStore(db)
	require.NoError(t, err)
	return patternStore, commentStore
}

func insertAnchorDriftEvent(t *testing.T, db *memorydb.SQLiteWorkflowStateStore, anchorID string) {
	t.Helper()
	_, err := db.DB().ExecContext(context.Background(), `
		INSERT INTO retrieval_anchor_events
		(event_id, anchor_id, event_type, detail, similarity_score, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "drift-"+anchorID, anchorID, "drift_detected", "meaning changed", 0.2, time.Now().UTC().Format(time.RFC3339))
	require.NoError(t, err)
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
