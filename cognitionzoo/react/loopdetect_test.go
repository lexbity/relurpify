package react

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
)

func TestLoopDetectorStallAtThresholdWithinWindow(t *testing.T) {
	detector := newLoopDetector()
	for i := 1; i <= loopStallThreshold-1; i++ {
		stall, _ := detector.record("cargo_test", json.RawMessage(`{"path":"."}`), "edit")
		if stall {
			t.Fatalf("iteration %d: stall before threshold", i)
		}
	}
	stall, sig := detector.record("cargo_test", json.RawMessage(`{"path":"."}`), "edit")
	if !stall {
		t.Fatal("expected stall at threshold")
	}
	if got := detector.count(sig); got != loopStallThreshold {
		t.Fatalf("count = %d, want %d", got, loopStallThreshold)
	}
}

func TestLoopDetectorDifferentActionResetsWindow(t *testing.T) {
	detector := newLoopDetector()
	for i := 0; i < loopStallThreshold-1; i++ {
		if stall, _ := detector.record("cargo_test", json.RawMessage(`{}`), "edit"); stall {
			t.Fatal("premature stall")
		}
	}
	// Intervening different actions dilute the window: repeated singles of a
	// new signature interleaved with others must not stall until the new
	// signature itself reaches the threshold within the last 12 actions.
	for i := 0; i < loopRingSize*2; i++ {
		if stall, _ := detector.record(fmt.Sprintf("tool_%d", i), json.RawMessage(`{}`), "explore"); stall {
			t.Fatalf("distinct tools stalled at iteration %d", i)
		}
	}
	stall, _ := detector.record("cargo_test", json.RawMessage(`{}`), "edit")
	if stall {
		t.Fatal("old signature must not stall after being evicted from the window")
	}
}

func TestLoopDetectorVaryingOutputIdenticalArgsStalls(t *testing.T) {
	// D7 regression: the detector hashes intent (tool+args+phase), never
	// output, so output variance cannot defeat detection.
	detector := newLoopDetector()
	for i := 0; i < loopStallThreshold; i++ {
		stall, _ := detector.record("cargo_test", json.RawMessage(`{"path":"."}`), "edit")
		if i < loopStallThreshold-1 && stall {
			t.Fatal("premature stall")
		}
		if i == loopStallThreshold-1 && !stall {
			t.Fatal("identical-arg repeats must stall regardless of outputs")
		}
	}
}

func TestLoopDetectorBoundedMemory(t *testing.T) {
	detector := newLoopDetector()
	for i := 0; i < loopRingSize*10; i++ {
		detector.record(fmt.Sprintf("tool_%d", i), json.RawMessage(`{}`), "explore")
	}
	if got := detector.ring.Len(); got != loopRingSize {
		t.Fatalf("ring len = %d, want %d", got, loopRingSize)
	}
	if len(detector.counts) > loopRingSize {
		t.Fatalf("counters map grew to %d entries", len(detector.counts))
	}
}

func TestLoopDetectorStateRoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	detector := loadLoopDetector(env)
	for i := 0; i < 3; i++ {
		detector.record("cargo_test", json.RawMessage(`{}`), "edit")
	}
	detector.recorded = 7
	saveLoopState(env, detector)

	restored := loadLoopDetector(env)
	stall, _ := restored.record("cargo_test", json.RawMessage(`{}`), "edit")
	if !stall {
		t.Fatal("restored detector lost window state")
	}
	if restored.recorded != 7 {
		t.Fatalf("recorded = %d, want 7", restored.recorded)
	}
}

func TestCanonicalJSONNormalizes(t *testing.T) {
	inputs := []string{`{"b":1,"a":2}`, `{ "a" : 2 }`, `{"a":{"z":true,"y":[3,1]}}`, `{}`}
	want := []string{`{"a":2,"b":1}`, `{"a":2}`, `{"a":{"y":[3,1],"z":true}}`, `{}`}
	for i, input := range inputs {
		if got := canonicalJSON(json.RawMessage(input)); got != want[i] {
			t.Fatalf("canonicalJSON(%s) = %s, want %s", input, got, want[i])
		}
	}
	// Key order and whitespace MUST not matter for signatures.
	sig1 := actionSignature("t", json.RawMessage(`{"b":1,"a":2}`), "p")
	sig2 := actionSignature("t", json.RawMessage(`{ "a" : 2, "b" : 1 }`), "p")
	if sig1 != sig2 {
		t.Fatal("key order / whitespace changed the action signature")
	}
	sig3 := actionSignature("t", json.RawMessage(`{"a":3,"b":1}`), "p")
	if sig1 == sig3 {
		t.Fatal("different args produced the same signature")
	}
	sig4 := actionSignature("t", json.RawMessage(`{"a":2,"b":1}`), "explore")
	if sig1 == sig4 {
		t.Fatal("different phase produced the same signature")
	}
}

func TestObserveLoopStallIndependentOfObservationTrimming(t *testing.T) {
	task := &execution.Task{ID: "task-1", Instruction: "analyze why the suite fails"}
	node := &reactObserveNode{id: "react_observe", agent: &ReActAgent{}, task: task}
	env := contextdata.NewEnvelope("task-1", "session-1")

	appendObservation := func(tool string, seq int) {
		history := getToolObservations(env)
		history = append(history, ToolObservation{
			Tool: tool, Phase: "edit", Seq: seq,
			Args:    map[string]any{"path": "."},
			Success: false,
		})
		env.SetWorkingValueWithClass("react.tool_observations", history, contextdata.MemoryClassTask)
	}

	for seq := 1; seq <= loopStallThreshold-1; seq++ {
		appendObservation("cargo_test", seq)
		node.observeLoopStall(context.Background(), env)
	}
	if guidance := envGetString(env, loopGuidanceKey); guidance != "" {
		t.Fatalf("guidance set before threshold: %q", guidance)
	}
	appendObservation("cargo_test", loopStallThreshold)
	node.observeLoopStall(context.Background(), env)
	if guidance := envGetString(env, loopGuidanceKey); guidance == "" {
		t.Fatal("expected stall guidance after threshold repeats")
	}

	// Simulate observation-history trimming: the detector's own state must
	// keep counting across it.
	trimmed := getToolObservations(env)
	last := trimmed[len(trimmed)-1]
	env.SetWorkingValueWithClass("react.tool_observations", []ToolObservation{last}, contextdata.MemoryClassTask)
	appendObservation("other_tool", loopStallThreshold+1)
	node.observeLoopStall(context.Background(), env)
	if guidance := envGetString(env, loopGuidanceKey); guidance != "" {
		t.Fatalf("an intervening different action must clear the stall, got %q", guidance)
	}
}

func TestObserveLoopStallVaryingOutputsStillDetected(t *testing.T) {
	task := &execution.Task{ID: "task-2", Instruction: "analyze the failure"}
	node := &reactObserveNode{id: "react_observe", agent: &ReActAgent{}, task: task}
	env := contextdata.NewEnvelope("task-2", "session-2")
	for seq := 1; seq <= loopStallThreshold; seq++ {
		history := getToolObservations(env)
		history = append(history, ToolObservation{
			Tool: "cargo_test", Phase: "edit", Seq: seq,
			Args: map[string]any{"path": "."},
			Data: map[string]any{"stdout": fmt.Sprintf("run %d: FAILED 0x%x", seq, seq*7919)},
		})
		env.SetWorkingValueWithClass("react.tool_observations", history, contextdata.MemoryClassTask)
		node.observeLoopStall(context.Background(), env)
	}
	if guidance := envGetString(env, loopGuidanceKey); guidance == "" {
		t.Fatal("varying outputs with identical args must stall (D7)")
	}
}
