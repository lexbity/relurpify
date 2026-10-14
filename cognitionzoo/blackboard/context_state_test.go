package blackboard

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	relurpctx "codeburg.org/lexbit/relurpify/context"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge/memory"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// recordingTelemetry captures emitted events.
type recordingTelemetry struct {
	events []telemetry.Event
}

func (r *recordingTelemetry) Emit(event telemetry.Event) {
	r.events = append(r.events, event)
}

func bbFullyPopulated(t *testing.T) *Blackboard {
	t.Helper()
	bb := NewBlackboard("goal one", "goal two")
	require.True(t, bb.AddFact("task.goal.0", "goal one", "Explorer"))
	require.NoError(t, bb.AddHypothesis("h1", "maybe this", 0.5, "Explorer"))
	require.NoError(t, bb.EnqueueAction(ActionRequest{
		ID: "a1", ToolOrAgent: "agent:react", Description: "d", RequestedBy: "Planner",
		Args: map[string]any{"k": "v"},
	}))
	require.NoError(t, bb.CompleteAction(ActionResult{RequestID: "a1", Success: true, Output: "out"}))
	require.NoError(t, bb.AddIssue("i1", "problem", "high", "Analyzer"))
	require.NoError(t, bb.AddArtifact("art1", "patch", "diff", "Executor"))
	return bb
}

func TestPublishAndLoadRoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	bb := bbFullyPopulated(t)

	PublishToContext(env, bb, ControllerState{Cycle: 2, MaxCycles: 20, Termination: "running", LastSource: "Analyzer"})

	loaded := LoadFromContext(env, "fallback goal")
	require.NotNil(t, loaded)
	assert.Equal(t, BlackboardStateSchemaVersion, loaded.SchemaVersion)
	assert.Equal(t, bb.Metrics(), loaded.Metrics())
	assert.Equal(t, []string{"goal one", "goal two"}, loaded.Goals)
	require.Len(t, loaded.Facts, 1)
	assert.Equal(t, "task.goal.0", loaded.Facts[0].Key)
	require.NotNil(t, loaded.Facts[0].Derivation, "publish stamps blackboard_inject derivation")
	require.Len(t, loaded.Hypotheses, 1)
	require.NotNil(t, loaded.Hypotheses[0].Derivation)
	require.Len(t, loaded.Issues, 1)
	require.NotNil(t, loaded.Issues[0].Derivation)
	require.Len(t, loaded.PendingActions, 0)
	require.Len(t, bb.CompletedActions, 1)
	assert.Equal(t, "a1", loaded.CompletedActions[0].RequestID)
	require.Len(t, loaded.Artifacts, 1)

	controller, ok := contextdata.GetTyped[ControllerState](env, contextKeyController)
	require.True(t, ok)
	assert.Equal(t, 2, controller.Cycle)
	assert.Equal(t, "running", controller.Termination)
	assert.False(t, controller.GoalSatisfied, "populated board has no verified artifact")
	assert.False(t, controller.LastUpdatedAt.IsZero())

	metrics, ok := contextdata.GetTyped[Metrics](env, contextKeyMetrics)
	require.True(t, ok)
	assert.Equal(t, bb.Metrics(), metrics)

	summaryText2, ok := contextdata.GetTyped[string](env, contextKeySummary)
	require.True(t, ok)
	assert.Contains(t, summaryText2, `goal="goal one"`)
	termination, ok := contextdata.GetTyped[string](env, contextKeyTermination)
	require.True(t, ok)
	assert.Equal(t, "running", termination)
	executionMode, ok := contextdata.GetTyped[string](env, contextKeyControllerExecutionMode)
	require.True(t, ok)
	assert.Equal(t, string(ExecutionModeSingleFireSerial), executionMode)
	selectionPolicy, ok := contextdata.GetTyped[string](env, contextKeyControllerSelectionPolicy)
	require.True(t, ok)
	assert.Equal(t, "highest_priority_then_name", selectionPolicy)
	mergePolicy, ok := contextdata.GetTyped[string](env, contextKeyControllerMergePolicy)
	require.True(t, ok)
	assert.Equal(t, string(BranchMergePolicyRejectConflicts), mergePolicy)

	executionSummaryValue, ok := contextdata.GetTyped[map[string]any](env, contextKeyExecutionSummary)
	require.True(t, ok)
	assert.Equal(t, "goal one", executionSummaryValue["goal"])
	assert.Equal(t, "running", executionSummaryValue["termination"])
}

func TestLoadFromContextEmptyAndNilState(t *testing.T) {
	bb := LoadFromContext(nil, "fallback")
	require.NotNil(t, bb)
	assert.Equal(t, []string{"fallback"}, bb.Goals)

	env := contextdata.NewEnvelope("t", "s")
	bb = LoadFromContext(env, "fallback")
	require.NotNil(t, bb)
	assert.Equal(t, []string{"fallback"}, bb.Goals)
}

func TestLoadFromContextBackfillsMissingGoals(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	bb := NewBlackboard()
	bb.Goals = nil
	require.True(t, bb.AddFact("k", "v", "s"))
	PublishToContext(env, bb, ControllerState{Termination: "running"})

	loaded := LoadFromContext(env, "injected goal")
	assert.Equal(t, []string{"injected goal"}, loaded.Goals, "empty published goals backfilled from loader arg")
}

func TestPublishToContextNilArgsAreNoOp(t *testing.T) {
	assert.NotPanics(t, func() {
		PublishToContext(nil, NewBlackboard(), ControllerState{})
		env := contextdata.NewEnvelope("t", "s")
		PublishToContext(env, nil, ControllerState{})
	})
}

func TestPublishToContextPanicsOnInvalidState(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	bb := &Blackboard{SchemaVersion: 1, Issues: []Issue{{ID: "dup", Severity: "low"}, {ID: "dup", Severity: "low"}}}
	assert.Panics(t, func() { PublishToContext(env, bb, ControllerState{}) })
}

func TestLoadFromContextRejectsUndecodableValues(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	envelopeSet(env, contextKeyGoals, make(chan int)) // not JSON-marshalable
	bb := LoadFromContext(env, "goal")
	assert.Equal(t, []string{"goal"}, bb.Goals, "undecodable lane falls back to a fresh board")
}

func TestLoadNamespacedBlackboardRejectsMismatchedShapes(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	envelopeSet(env, contextKeyGoals, "not-a-list")
	envelopeSet(env, contextKeyFacts, "not-a-list")
	loaded, ok := loadNamespacedBlackboard(env)
	assert.False(t, ok)
	assert.Nil(t, loaded)
}

func TestMirrorBlackboardToWorkingMemoryStore(t *testing.T) {
	store := memory.NewWorkingMemoryStore()
	env := contextdata.NewEnvelope("task-mirror", "session-1")
	env.SetWorkingValueWithClass(contextKeyWorkingMemoryStore, store, contextdata.MemoryClassTask)
	bb := bbFullyPopulated(t)
	PublishToContext(env, bb, ControllerState{Cycle: 1, MaxCycles: 20, Termination: "running"})

	scope := store.Scope("task-mirror")
	require.NotNil(t, scope)
	entry, ok := scope.Get(contextKeyGoals)
	require.True(t, ok, "goals mirrored into working memory store")
	goals, ok := entry.Value.([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"goal one", "goal two"}, goals)

	_, ok = scope.Get(contextKeySummary)
	assert.True(t, ok)
}

func TestMirrorBlackboardWithoutStoreOrTaskID(t *testing.T) {
	assert.NotPanics(t, func() {
		env := contextdata.NewEnvelope("", "")
		mirrorBlackboardToWorkingMemoryStore(env)
		env.SetWorkingValueWithClass(contextKeyWorkingMemoryStore, "not-a-store", contextdata.MemoryClassTask)
		env.SetTaskID("task-x")
		mirrorBlackboardToWorkingMemoryStore(env)
	})
}

func TestHydrateBlackboardFromMemoryPayloadVariants(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	bb := NewBlackboard("g")

	// No memory payloads -> nothing added.
	assert.Equal(t, 0, hydrateBlackboardFromMemory(env, bb))

	env.SetWorkingValueWithClass("graph.declarative_memory_payload", map[string]any{
		"results": []map[string]any{
			{"record_id": "r1", "summary": "fact one", "source": "docs"},
			{"record_id": "  ", "summary": "", "text": "text fallback", "source": "arch"},
			// NOTE: a record with both summary and text missing formats the
			// absent text as "<nil>", which defeats the skip guard (recorded
			// bug in hydrateBlackboardFromMemory).
			{"record_id": "r3", "summary": "  "},
		},
	}, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("graph.procedural_memory_payload", map[string]any{
		"results": []map[string]any{
			{"record_id": "p1", "summary": "routine one", "source": "learned"},
		},
	}, contextdata.MemoryClassTask)
	added := hydrateBlackboardFromMemory(env, bb)
	assert.Equal(t, 4, added)

	fact, ok := findFact(bb, "memory:r1")
	require.True(t, ok)
	assert.Equal(t, "fact one", fact.Value)
	assert.Equal(t, "memory:docs", fact.Source)
	require.NotNil(t, fact.Origin)
	assert.Equal(t, "docs", fact.Origin.SourceSystem)
	assert.Equal(t, "r1", fact.Origin.RecordID)
	require.NotNil(t, fact.Derivation)

	_, ok = findFact(bb, "memory:")
	require.True(t, ok, "blank record id and summary leave an empty key; text becomes the value")

	r3, ok := findFact(bb, "memory:r3")
	require.True(t, ok)
	assert.Equal(t, "<nil>", r3.Value, "documents the missing-text <nil> behavior")

	hyp, ok := func() (Hypothesis, bool) {
		for _, h := range bb.Hypotheses {
			if h.ID == "memory:routine:p1" {
				return h, true
			}
		}
		return Hypothesis{}, false
	}()
	require.True(t, ok)
	assert.Equal(t, 0.55, hyp.Confidence)
	assert.Equal(t, "memory:learned", hyp.Source)
	require.NotNil(t, hyp.Origin)
}

func TestHydrateBlackboardFromMemoryEnvelopeRecords(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	bb := NewBlackboard("g")
	env.SetWorkingValueWithClass("graph.declarative_memory", map[string]any{
		"results": []relurpctx.MemoryRecordEnvelope{
			{Key: "dec-key", Summary: " decision ", RecordID: "dr1", Scope: "repo"},
		},
	}, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("graph.procedural_memory", map[string]any{
		"results": []relurpctx.MemoryRecordEnvelope{
			{Key: "proc-key", RecordID: "pr1", Scope: "global"}, // empty summary -> default description
		},
	}, contextdata.MemoryClassTask)

	added := hydrateBlackboardFromMemory(env, bb)
	assert.Equal(t, 2, added)

	fact, ok := findFact(bb, "memory:dec-key")
	require.True(t, ok)
	assert.Equal(t, "decision", fact.Value)
	assert.Equal(t, "memory:declarative", fact.Source)
	require.NotNil(t, fact.Origin)
	assert.Equal(t, "declarative", fact.Origin.SourceSystem)
	assert.Equal(t, "dr1", fact.Origin.RecordID)
	assert.Equal(t, "repo", fact.Origin.Scope)

	hyp := bb.Hypotheses[0]
	assert.Equal(t, "memory:routine:proc-key", hyp.ID)
	assert.Equal(t, "consider learned routine proc-key", hyp.Description)
	require.NotNil(t, hyp.Origin)
	assert.Equal(t, "procedural", hyp.Origin.SourceSystem)
}

func TestHydrateNilArgs(t *testing.T) {
	assert.Equal(t, 0, hydrateBlackboardFromMemory(nil, NewBlackboard()))
	assert.Equal(t, 0, hydrateBlackboardFromMemory(contextdata.NewEnvelope("t", "s"), nil))
}

func TestPublishPersistenceCandidatesSummaryAndRoutine(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	bb := NewBlackboard("ship it")
	metrics := bb.Metrics()
	publishPersistenceCandidates(env, bb, ControllerState{Cycle: 3, MaxCycles: 20, Termination: "running"}, metrics)

	summary, ok := contextdata.GetTyped[map[string]any](env, contextKeyPersistenceSummary)
	require.True(t, ok)
	assert.Equal(t, false, summary["verified"])
	summaryResult := summary["result"].(map[string]any)
	assert.Equal(t, "ship it", summaryResult["goal"])

	decision, ok := contextdata.GetTyped[map[string]any](env, contextKeyPersistenceDecision)
	require.True(t, ok)
	assert.Equal(t, "blackboard terminated with running after 3 cycles", decision["summary"])

	routine, ok := contextdata.GetTyped[map[string]any](env, contextKeyPersistenceRoutine)
	require.True(t, ok)
	assert.Empty(t, routine, "no routine without a satisfied verified goal")
}

func TestPublishPersistenceCandidatesGoalSatisfiedEmitsRoutine(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	bb := NewBlackboard("ship it")
	require.NoError(t, bb.AddArtifact("a", "patch", "diff", "Executor"))
	bb.VerifyArtifact("a")
	require.NoError(t, bb.CompleteAction(ActionResult{RequestID: "r1"}))
	controller := ControllerState{Cycle: 2, MaxCycles: 20, Termination: "goal_satisfied", GoalSatisfied: true}
	publishPersistenceCandidates(env, bb, controller, bb.Metrics())

	decision, _ := contextdata.GetTyped[map[string]any](env, contextKeyPersistenceDecision)
	assert.Equal(t, `blackboard satisfied "ship it" after 2 cycles`, decision["summary"])

	routine, ok := contextdata.GetTyped[map[string]any](env, contextKeyPersistenceRoutine)
	require.True(t, ok)
	assert.Equal(t, "blackboard execution routine", routine["name"])
	assert.Equal(t, true, routine["verified"])
}

func TestSummaryTextAndExecutionSummaryNilSafety(t *testing.T) {
	assert.Equal(t, "", summaryText(nil, Metrics{}))
	bb := NewBlackboard("g", "g2")
	text := summaryText(bb, bb.Metrics())
	assert.Equal(t, `goal="g" facts=0 issues=0 pending=0 completed=0 artifacts=0 verified=0`, text)

	summary := executionSummary(nil, ControllerState{Termination: "stuck"}, Metrics{FactCount: 1})
	assert.Equal(t, "", summary["goal"])
	assert.Equal(t, "stuck", summary["termination"])
}

func TestDecodeContextValue(t *testing.T) {
	var out map[string]any
	assert.True(t, decodeContextValue(map[string]any{"a": 1}, &out))
	assert.Equal(t, float64(1), out["a"])
	assert.False(t, decodeContextValue(make(chan int), &out))
}

func TestMergeBlackboardBranches(t *testing.T) {
	t.Run("nil parent or no branches is a no-op", func(t *testing.T) {
		assert.NoError(t, MergeBlackboardBranches(nil, nil))
		parent := contextdata.NewEnvelope("t", "s")
		assert.NoError(t, MergeBlackboardBranches(parent, nil))
	})
	t.Run("nil branch state errors", func(t *testing.T) {
		parent := contextdata.NewEnvelope("t", "s")
		err := MergeBlackboardBranches(parent, []BlackboardBranchResult{{SourceName: "a"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires isolated state")
	})
	t.Run("conflicting writes rejected", func(t *testing.T) {
		parent := contextdata.NewEnvelope("t", "s")
		left := contextdata.NewEnvelope("t", "s")
		left.SetWorkingValue("conflict.key", "left")
		right := contextdata.NewEnvelope("t", "s")
		right.SetWorkingValue("conflict.key", "right")
		err := MergeBlackboardBranches(parent, []BlackboardBranchResult{
			{SourceName: "left", State: left},
			{SourceName: "right", State: right},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `written by both "left" and "right"`)
	})
	t.Run("disjoint branches merge and reactive runtime state", func(t *testing.T) {
		parent := contextdata.NewEnvelope("t", "s")
		left := contextdata.NewEnvelope("t", "s")
		left.SetWorkingValue("left.key", "l")
		right := contextdata.NewEnvelope("t", "s")
		right.SetWorkingValue("right.key", "r")
		require.NoError(t, MergeBlackboardBranches(parent, []BlackboardBranchResult{
			{State: left}, // empty source name -> "unknown" label
			{SourceName: "right", State: right},
		}))
		assert.Equal(t, "l", envelopeGetString(parent, "left.key"))
		assert.Equal(t, "r", envelopeGetString(parent, "right.key"))
		require.NotNil(t, LoadFromContext(parent, "goal"), "runtime active state refreshed after merge")
		active, err := activeBlackboard(parent)
		require.NoError(t, err)
		// The reactive board is loaded with the empty task instruction, which
		// NewBlackboard stores as a single empty-string goal.
		assert.Equal(t, []string{""}, active.Goals)
	})
}

func TestAppendBlackboardAuditCapsEntries(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	for i := 0; i < maxBlackboardAuditEntries+5; i++ {
		appendBlackboardAudit(env, "entry", map[string]any{"i": i})
	}
	raw, ok := envelopeGet(env, contextKeyAuditTrail)
	require.True(t, ok)
	entries, ok := raw.([]map[string]any)
	require.True(t, ok)
	assert.Len(t, entries, maxBlackboardAuditEntries)
}

func TestEmitBlackboardEventAppendsAuditAndSink(t *testing.T) {
	sink := &recordingTelemetry{}
	env := contextdata.NewEnvelope("task-emit", "session-1")
	emitBlackboardEvent(context.Background(), sink, env, telemetry.EventStateChange, "node-1", "task-emit", " something happened ", map[string]any{"k": "v"})
	require.Len(t, sink.events, 1)
	event := sink.events[0]
	assert.Equal(t, telemetry.EventStateChange, event.Type)
	assert.Equal(t, "node-1", event.NodeID)
	assert.Equal(t, "something happened", event.Message, "message trimmed")
	assert.Equal(t, "v", event.Metadata["k"])

	raw, ok := envelopeGet(env, contextKeyAuditTrail)
	require.True(t, ok)
	entries := raw.([]map[string]any)
	require.Len(t, entries, 1)
	assert.Equal(t, "something happened", entries[0]["message"])

	// Nil sink: audit still recorded, no panic.
	emitBlackboardEvent(context.Background(), nil, env, telemetry.EventStateChange, "node-1", "task-emit", "second", nil)
	raw, _ = envelopeGet(env, contextKeyAuditTrail)
	assert.Len(t, raw.([]map[string]any), 2)
}

func TestCloneTelemetryMetadata(t *testing.T) {
	assert.Nil(t, cloneTelemetryMetadata(nil))
	src := map[string]any{"a": 1}
	clone := cloneTelemetryMetadata(src)
	assert.Equal(t, src, clone)
	clone["a"] = 2
	assert.Equal(t, 1, src["a"])
}

func TestActiveBlackboardVariants(t *testing.T) {
	_, err := activeBlackboard(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "state required")

	// An empty envelope yields a fresh board (LoadFromContext never returns
	// nil for a non-nil state, so the explicit nil check in activeBlackboard
	// is defensive dead code).
	env := contextdata.NewEnvelope("t", "s")
	fresh, err := activeBlackboard(env)
	require.NoError(t, err)
	assert.Equal(t, BlackboardStateSchemaVersion, fresh.SchemaVersion)

	// Existing active blackboard is normalized and returned.
	active := &Blackboard{}
	envelopeSet(env, contextKeyRuntimeActive, active)
	bb, err := activeBlackboard(env)
	require.NoError(t, err)
	assert.Equal(t, BlackboardStateSchemaVersion, bb.SchemaVersion)
	env2 := contextdata.NewEnvelope("t", "s")
	envelopeSet(env2, contextKeyRuntimeActive, "not-a-board")
	envelopeSet(env2, contextKeyGoals, []string{"from-context"})
	bb, err = activeBlackboard(env2)
	require.NoError(t, err)
	assert.Equal(t, []string{"from-context"}, bb.Goals)

	// Typed non-pointer garbage -> LoadFromContext path still yields a board.
	env3 := contextdata.NewEnvelope("t", "s")
	envelopeSet(env3, contextKeyGoals, []string{"g"})
	bb, err = activeBlackboard(env3)
	require.NoError(t, err)
	assert.Equal(t, []string{"g"}, bb.Goals)
}

func TestCurrentCycle(t *testing.T) {
	assert.Equal(t, 0, currentCycle(nil))
	env := contextdata.NewEnvelope("t", "s")
	assert.Equal(t, 0, currentCycle(env))
	envelopeSet(env, contextKeyControllerCycle, 4)
	assert.Equal(t, 4, currentCycle(env))
	envelopeSet(env, contextKeyControllerCycle, "not-an-int")
	assert.Equal(t, 0, currentCycle(env))
}

func TestLoadNodeExecutesAndPublishes(t *testing.T) {
	store := memory.NewWorkingMemoryStore()
	env := contextdata.NewEnvelope("task-load", "session-1")
	env.SetWorkingValueWithClass(contextKeyWorkingMemoryStore, store, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("task.id", "task-load", contextdata.MemoryClassTask)
	sink := &recordingTelemetry{}
	node := &blackboardLoadNode{id: "bb_load", goal: "the goal", maxCycles: 12, taskID: "task-load", store: store, telemetry: sink}

	result, err := node.Execute(context.Background(), env)
	require.NoError(t, err)
	assert.True(t, result.Success)

	active, err := activeBlackboard(env)
	require.NoError(t, err)
	assert.Equal(t, []string{"the goal"}, active.Goals)
	controller, ok := contextdata.GetTyped[ControllerState](env, contextKeyController)
	require.True(t, ok)
	assert.Equal(t, "running", controller.Termination)
	assert.Equal(t, 12, controller.MaxCycles)
	require.NotEmpty(t, sink.events)

	// Retrieve from the store exercises hydrateBlackboardFromWorkingMemoryStore on a second run.
	_, err = node.Execute(context.Background(), env)
	require.NoError(t, err)
}

func TestLoadNodeWithoutStoreOrTaskID(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	node := &blackboardLoadNode{id: "bb_load", goal: "g", maxCycles: 5, taskID: "   "}
	result, err := node.Execute(context.Background(), env)
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestEvaluateNodeGoalSatisfied(t *testing.T) {
	env := contextdata.NewEnvelope("task-eval", "s")
	env.SetWorkingValueWithClass("task.id", "task-eval", contextdata.MemoryClassTask)
	bb := NewBlackboard("goal")
	require.NoError(t, bb.AddArtifact("a", "patch", "c", "s"))
	bb.VerifyArtifact("a")
	envelopeSet(env, contextKeyRuntimeActive, bb)
	PublishToContext(env, bb, ControllerState{Cycle: 0, MaxCycles: 5, Termination: "running"})
	envelopeSet(env, contextKeyControllerCycle, 1)

	node := &blackboardEvaluateNode{id: "bb_evaluate", controller: &Controller{Sources: DefaultKnowledgeSources(), MaxCycles: 5}}
	result, err := node.Execute(context.Background(), env)
	require.NoError(t, err)
	assert.Equal(t, "bb_done", result.Data.Fields()["next"])
	controller, _ := contextdata.GetTyped[ControllerState](env, contextKeyController)
	assert.Equal(t, "goal_satisfied", controller.Termination)
	assert.Equal(t, "bb_done", envelopeGetString(env, contextKeyControllerNext))
}

func TestEvaluateNodeCycleLimit(t *testing.T) {
	env := contextdata.NewEnvelope("task-eval", "s")
	bb := NewBlackboard("goal")
	envelopeSet(env, contextKeyRuntimeActive, bb)
	envelopeSet(env, contextKeyControllerCycle, 3)

	node := &blackboardEvaluateNode{id: "bb_evaluate", controller: &Controller{Sources: DefaultKnowledgeSources(), MaxCycles: 3}}
	result, err := node.Execute(context.Background(), env)
	require.NoError(t, err)
	assert.Equal(t, "bb_done", result.Data.Fields()["next"])
	controller, _ := contextdata.GetTyped[ControllerState](env, contextKeyController)
	assert.Equal(t, "cycle_limit", controller.Termination)
}

func TestEvaluateNodeStuck(t *testing.T) {
	env := contextdata.NewEnvelope("task-eval", "s")
	bb := NewBlackboard("goal")
	envelopeSet(env, contextKeyRuntimeActive, bb)

	// Empty source set: nothing can activate.
	node := &blackboardEvaluateNode{id: "bb_evaluate", controller: &Controller{Sources: nil, MaxCycles: 3}}
	result, err := node.Execute(context.Background(), env)
	require.NoError(t, err)
	assert.Equal(t, "bb_done", result.Data.Fields()["next"])
	controller, _ := contextdata.GetTyped[ControllerState](env, contextKeyController)
	assert.Equal(t, "stuck", controller.Termination)
	eligible, ok := contextdata.GetTyped[[]string](env, contextKeyControllerEligible)
	require.True(t, ok)
	assert.Empty(t, eligible)
}

func TestEvaluateNodeSelectsSource(t *testing.T) {
	env := contextdata.NewEnvelope("task-eval", "s")
	bb := NewBlackboard("goal")
	envelopeSet(env, contextKeyRuntimeActive, bb)

	node := &blackboardEvaluateNode{id: "bb_evaluate", controller: &Controller{Sources: DefaultKnowledgeSources(), MaxCycles: 5}}
	result, err := node.Execute(context.Background(), env)
	require.NoError(t, err)
	assert.Equal(t, "bb_dispatch", result.Data.Fields()["next"])
	assert.Equal(t, "Explorer", result.Data.Fields()["selected_source"])
	assert.Equal(t, 1, result.Data.Fields()["cycle"])
	assert.Equal(t, "bb_dispatch", envelopeGetString(env, contextKeyControllerNext))
	assert.Equal(t, "Explorer", envelopeGetString(env, contextKnowledgeLastSource))
	eligible, ok := contextdata.GetTyped[[]string](env, contextKeyControllerEligible)
	require.True(t, ok)
	assert.Equal(t, "Explorer", eligible[0])
	controller, _ := contextdata.GetTyped[ControllerState](env, contextKeyController)
	assert.Equal(t, "running", controller.Termination)
	assert.Equal(t, "Explorer", controller.LastSource)
}

func TestDispatchNodeExecutesSelectedSource(t *testing.T) {
	env := contextdata.NewEnvelope("task-dispatch", "s")
	bb := NewBlackboard("goal")
	envelopeSet(env, contextKeyRuntimeActive, bb)
	envelopeSet(env, contextKeyControllerCycle, 1)
	envelopeSet(env, contextKnowledgeLastSource, "Explorer")
	sink := &recordingTelemetry{}

	controller := &Controller{Sources: DefaultKnowledgeSources(), MaxCycles: 5}
	node := &blackboardDispatchNode{id: "bb_dispatch", controller: controller, telemetry: sink}
	result, err := node.Execute(context.Background(), env)
	require.NoError(t, err)
	assert.Equal(t, "Explorer", result.Data.Fields()["source"])
	assert.True(t, bb.HasFact("exploration.status"), "dispatch executed the Explorer source against the active board")
	require.NotEmpty(t, sink.events)

	controllerState, _ := contextdata.GetTyped[ControllerState](env, contextKeyController)
	assert.Equal(t, "running", controllerState.Termination)
	assert.Equal(t, "Explorer", controllerState.LastSource)
}

func TestDispatchNodeMissingSourceName(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	envelopeSet(env, contextKeyRuntimeActive, NewBlackboard())
	node := &blackboardDispatchNode{id: "bb_dispatch", controller: &Controller{Sources: DefaultKnowledgeSources()}}
	_, err := node.Execute(context.Background(), env)
	require.EqualError(t, err, "blackboard: dispatch source missing")
}

func TestDispatchNodeUnknownSource(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	envelopeSet(env, contextKeyRuntimeActive, NewBlackboard())
	envelopeSet(env, contextKnowledgeLastSource, "Ghost")
	node := &blackboardDispatchNode{id: "bb_dispatch", controller: &Controller{Sources: DefaultKnowledgeSources()}}
	_, err := node.Execute(context.Background(), env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `knowledge source "Ghost" not found`)
}

func TestDispatchNodeSourceErrorRecordsFailure(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	envelopeSet(env, contextKeyRuntimeActive, NewBlackboard())
	envelopeSet(env, contextKnowledgeLastSource, "Boom")
	envelopeSet(env, contextKeyControllerCycle, 2)
	failing := &stubKS{name: "Boom", execErr: context.Canceled}
	node := &blackboardDispatchNode{id: "bb_dispatch", controller: &Controller{Sources: []KnowledgeSource{failing}}}
	_, err := node.Execute(context.Background(), env)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, context.Canceled.Error(), envelopeGetString(env, contextKeyControllerLastError))
	controllerState, ok := contextdata.GetTyped[ControllerState](env, contextKeyController)
	require.True(t, ok)
	assert.Equal(t, "dispatch_error", controllerState.Termination)
}

func TestHydrateBlackboardFromWorkingMemoryStoreVariants(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	// Nil store / empty task id are no-ops.
	require.NoError(t, hydrateBlackboardFromWorkingMemoryStore(context.Background(), env, nil, "task"))
	require.NoError(t, hydrateBlackboardFromWorkingMemoryStore(context.Background(), env, memory.NewWorkingMemoryStore(), "  "))

	// Store without matching task returns no results; hydration is a no-op.
	store := memory.NewWorkingMemoryStore()
	require.NoError(t, hydrateBlackboardFromWorkingMemoryStore(context.Background(), env, store, "task-unknown"))
}

func TestWorkingMemoryStoreRoundTripThroughLoadNode(t *testing.T) {
	store := memory.NewWorkingMemoryStore()
	env := contextdata.NewEnvelope("task-rt", "s")
	env.SetWorkingValueWithClass(contextKeyWorkingMemoryStore, store, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("task.id", "task-rt", contextdata.MemoryClassTask)
	node := &blackboardLoadNode{id: "bb_load", goal: "round trip", maxCycles: 5, taskID: "task-rt", store: store}
	_, err := node.Execute(context.Background(), env)
	require.NoError(t, err)
	// The mirror wrote blackboard.goals into the task scope.
	scope := store.Scope("task-rt")
	entry, ok := scope.Get(contextKeyGoals)
	require.True(t, ok)
	goals, ok := entry.Value.([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"round trip"}, goals)
}
