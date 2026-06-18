package euclotui

import (
	"encoding/json"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	"codeburg.org/lexbit/relurpify/telemetry"
)

func TestExecutionEventFromTelemetry_EucloLifecycle(t *testing.T) {
	tests := []struct {
		name  string
		typ   telemetry.EventType
		meta  map[string]any
		check func(*testing.T, ExecutionEvent)
	}{
		{
			name: "step.started",
			typ:  "euclo.step.started",
			meta: map[string]any{
				"step_id":  "s1",
				"index":    float64(1),
				"total":    float64(3),
				"paradigm": "goalcon",
			},
			check: func(t *testing.T, ev ExecutionEvent) {
				if ev.Type != reporting.EventTypeStepStartedEuclo {
					t.Fatalf("type = %s, want %s", ev.Type, reporting.EventTypeStepStartedEuclo)
				}
				if ev.StepID != "s1" {
					t.Fatalf("StepID = %q, want s1", ev.StepID)
				}
				if ev.Index != 1 {
					t.Fatalf("Index = %d, want 1", ev.Index)
				}
				if ev.Total != 3 {
					t.Fatalf("Total = %d, want 3", ev.Total)
				}
				if ev.Paradigm != "goalcon" {
					t.Fatalf("Paradigm = %q, want goalcon", ev.Paradigm)
				}
			},
		},
		{
			name: "step.completed",
			typ:  "euclo.step.completed",
			meta: map[string]any{
				"step_id":     "s2",
				"success":     true,
				"duration_ms": float64(1234),
				"index":       float64(2),
				"total":       float64(3),
			},
			check: func(t *testing.T, ev ExecutionEvent) {
				if ev.Type != reporting.EventTypeStepCompletedEuclo {
					t.Fatalf("type = %s, want %s", ev.Type, reporting.EventTypeStepCompletedEuclo)
				}
				if ev.StepID != "s2" {
					t.Fatalf("StepID = %q, want s2", ev.StepID)
				}
				if !ev.Success {
					t.Fatal("expected Success=true")
				}
				if ev.DurationMs != 1234 {
					t.Fatalf("DurationMs = %d, want 1234", ev.DurationMs)
				}
			},
		},
		{
			name: "recipe.selected",
			typ:  "euclo.recipe.selected",
			meta: map[string]any{
				"recipe_id": "test.generate_tests",
			},
			check: func(t *testing.T, ev ExecutionEvent) {
				if ev.Type != reporting.EventTypeRecipeSelected {
					t.Fatalf("type = %s, want %s", ev.Type, reporting.EventTypeRecipeSelected)
				}
				if ev.RecipeID != "test.generate_tests" {
					t.Fatalf("RecipeID = %q", ev.RecipeID)
				}
			},
		},
		{
			name: "branch.resolved with skipped_step_ids as []any",
			typ:  "euclo.branch.resolved",
			meta: map[string]any{
				"skipped_step_ids": []any{"else.path", "default.path"},
				"chosen_branch":    "if.path",
			},
			check: func(t *testing.T, ev ExecutionEvent) {
				if ev.Type != reporting.EventTypeBranchResolved {
					t.Fatalf("type = %s, want %s", ev.Type, reporting.EventTypeBranchResolved)
				}
				skipped, ok := ev.Payload["skipped_step_ids"].([]string)
				if !ok {
					t.Fatal("Payload[skipped_step_ids] is not []string")
				}
				if len(skipped) != 2 || skipped[0] != "else.path" || skipped[1] != "default.path" {
					t.Fatalf("skipped = %v, want [else.path default.path]", skipped)
				}
			},
		},
		{
			name: "branch.resolved with skipped_step_ids as []string",
			typ:  "euclo.branch.resolved",
			meta: map[string]any{
				"skipped_step_ids": []string{"direct.skip"},
			},
			check: func(t *testing.T, ev ExecutionEvent) {
				skipped, ok := ev.Payload["skipped_step_ids"].([]string)
				if !ok {
					t.Fatal("Payload[skipped_step_ids] is not []string")
				}
				if len(skipped) != 1 || skipped[0] != "direct.skip" {
					t.Fatalf("skipped = %v, want [direct.skip]", skipped)
				}
			},
		},
		{
			name: "branch.resolved with no skipping — nil skipped_step_ids",
			typ:  "euclo.branch.resolved",
			meta: map[string]any{
				"chosen_branch": "main",
			},
			check: func(t *testing.T, ev ExecutionEvent) {
				if ev.Payload != nil {
					if _, exists := ev.Payload["skipped_step_ids"]; exists {
						t.Fatal("skipped_step_ids should not be set when nil")
					}
				}
			},
		},
		{
			name: "intake.complete",
			typ:  "euclo.intake.complete",
			meta: map[string]any{
				"confidence":       float64(0.95),
				"capability_count": float64(12),
			},
			check: func(t *testing.T, ev ExecutionEvent) {
				if ev.Type != reporting.EventTypeIntakeComplete {
					t.Fatalf("type = %s, want %s", ev.Type, reporting.EventTypeIntakeComplete)
				}
			},
		},
		{
			name: "route.selected",
			typ:  "euclo.route.selected",
			meta: map[string]any{
				"route_id":        "capability.test",
				"candidate_count": float64(3),
			},
			check: func(t *testing.T, ev ExecutionEvent) {
				if ev.Type != reporting.EventTypeRouteSelected {
					t.Fatalf("type = %s, want %s", ev.Type, reporting.EventTypeRouteSelected)
				}
			},
		},
		{
			name: "projection.completed",
			typ:  "euclo.projection.completed",
			meta: map[string]any{
				"plan_id": "plan-1",
			},
			check: func(t *testing.T, ev ExecutionEvent) {
				if ev.Type != reporting.EventTypeProjectionCompleted {
					t.Fatalf("type = %s", ev.Type)
				}
			},
		},
		{
			name: "non-euclo type is ignored",
			typ:  "graph_start",
			meta: map[string]any{},
			check: func(_ *testing.T, _ ExecutionEvent) {
				// Should not reach here — ok=false
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev := telemetry.Event{
				Type:      tc.typ,
				TaskID:    "test-task",
				Timestamp: time.Now().UTC(),
				Metadata:  tc.meta,
			}
			result, ok := ExecutionEventFromTelemetry(ev)
			if tc.name == "non-euclo type is ignored" {
				if ok {
					t.Fatal("expected ok=false for non-euclo event")
				}
				return
			}
			if !ok {
				t.Fatalf("expected ok=true for %s", tc.typ)
			}
			if result.TaskID != "test-task" {
				t.Fatalf("TaskID = %q, want test-task", result.TaskID)
			}
			tc.check(t, result)
		})
	}
}

func TestExecutionEventFromTelemetry_ToolEdited(t *testing.T) {
	ev := telemetry.Event{
		Type:      telemetry.EventToolEdited,
		TaskID:    "tool-task",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"path":          "demo.txt",
			"step_id":       "edit-step",
			"origin":        "file_edit",
			"lines_added":   float64(10),
			"lines_removed": float64(2),
			"preview":       "@@ -1 +1,2 @@\n-old\n+new\n",
		},
	}
	result, ok := ExecutionEventFromTelemetry(ev)
	if !ok {
		t.Fatal("expected ok=true for tool.edited")
	}
	if len(result.PatchHunks) != 1 {
		t.Fatalf("expected 1 PatchHunk, got %d", len(result.PatchHunks))
	}
	h := result.PatchHunks[0]
	if h.File != "demo.txt" {
		t.Fatalf("File = %q, want demo.txt", h.File)
	}
	if h.StepID != "edit-step" {
		t.Fatalf("StepID = %q, want edit-step", h.StepID)
	}
	if h.Origin != "file_edit" {
		t.Fatalf("Origin = %q, want file_edit", h.Origin)
	}
	if h.LinesAdded != 10 {
		t.Fatalf("LinesAdded = %d, want 10", h.LinesAdded)
	}
	if h.LinesRemoved != 2 {
		t.Fatalf("LinesRemoved = %d, want 2", h.LinesRemoved)
	}
	if h.Body != "@@ -1 +1,2 @@\n-old\n+new\n" {
		t.Fatalf("Body = %q", h.Body)
	}
}

func TestExecutionEventFromTelemetry_ToolEditedNoPathIsIgnored(t *testing.T) {
	ev := telemetry.Event{
		Type: telemetry.EventToolEdited,
		Metadata: map[string]any{
			"origin": "file_write",
		},
	}
	result, ok := ExecutionEventFromTelemetry(ev)
	if ok {
		t.Fatal("expected ok=false for tool.edited without path")
	}
	if len(result.PatchHunks) != 0 {
		t.Fatal("expected no PatchHunks")
	}
}

func TestExecEventApplier_SeqGuardDropsReplay(t *testing.T) {
	router := NewEucloEventRouter()
	applier := NewExecEventApplier(router)

	// Apply step.started with Seq=1.
	ev1 := telemetry.Event{
		Type:      "euclo.step.started",
		TaskID:    "task-1",
		Seq:       1,
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]any{"step_id": "s1", "index": float64(1), "total": float64(3)},
	}
	snap1, ok1 := applier.Apply(ev1)
	if !ok1 {
		t.Fatal("expected first apply to be accepted")
	}
	if snap1.StepRuntime["s1"].Status != surface.StepActive {
		t.Fatalf("status = %v, want StepActive", snap1.StepRuntime["s1"].Status)
	}

	// Re-apply same event (Seq=1 ≤ highWater=1) → should be dropped.
	snap1b, ok1b := applier.Apply(ev1)
	if ok1b {
		t.Fatal("expected replayed event to be dropped")
	}
	if snap1b.StepRuntime["s1"].Status != surface.StepActive {
		t.Fatal("replay should not change status")
	}

	// Apply step.completed with Seq=2 for same step.
	ev2 := telemetry.Event{
		Type:      "euclo.step.completed",
		TaskID:    "task-1",
		Seq:       2,
		Timestamp: time.Now().UTC(),
		Metadata:  map[string]any{"step_id": "s1", "success": true, "duration_ms": float64(5)},
	}
	snap2, ok2 := applier.Apply(ev2)
	if !ok2 {
		t.Fatal("expected completed to be accepted")
	}
	if snap2.StepRuntime["s1"].Status != surface.StepDone {
		t.Fatalf("status = %v, want StepDone", snap2.StepRuntime["s1"].Status)
	}
}

func TestExecEventApplier_NilRouter(t *testing.T) {
	var nilApplier *ExecEventApplier
	snap, ok := nilApplier.Apply(telemetry.Event{})
	if ok {
		t.Fatal("expected ok=false for nil applier")
	}
	if len(snap.Chat.Milestones) > 0 {
		t.Fatal("expected zero snapshot")
	}
}

func TestExecEventApplier_SkipsNonEuclo(t *testing.T) {
	router := NewEucloEventRouter()
	applier := NewExecEventApplier(router)

	ev := telemetry.Event{
		Type:      "graph_start",
		Timestamp: time.Now().UTC(),
	}
	_, ok := applier.Apply(ev)
	if ok {
		t.Fatal("expected ok=false for non-euclo event")
	}
}

// TestBridge_JSONRoundTripBranchResolved simulates the real telemetry path:
// reporting.EventBranchResolved → json.Marshal → json.Unmarshal → map[string]any
// → ExecutionEventFromTelemetry → router.ApplyExecutionEvent → skipped steps marked.
func TestBridge_JSONRoundTripBranchResolved(t *testing.T) {
	branchEv := reporting.EventBranchResolved{
		EventHeader: reporting.EventHeader{
			TaskID: "task-json",
		},
		GroupID:        "group-1",
		ChosenBranch:   "if.path",
		SkippedStepIDs: []string{"else.path", "default.path"},
	}

	raw, err := json.Marshal(branchEv)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	meta := make(map[string]any)
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Pre-seed step runtime for the skipped steps so they get the SKIPPED status.
	router := NewEucloEventRouter()
	router.stepRuntime["else.path"] = surface.StepRuntime{StepID: "else.path", Status: surface.StepActive}
	router.stepRuntime["default.path"] = surface.StepRuntime{StepID: "default.path", Status: surface.StepActive}
	applier := NewExecEventApplier(router)

	telEv := telemetry.Event{
		Type:      "euclo.branch.resolved",
		TaskID:    "task-json",
		Timestamp: time.Now().UTC(),
		Metadata:  meta,
	}
	snap, ok := applier.Apply(telEv)
	if !ok {
		t.Fatal("expected ok=true for branch.resolved via JSON round-trip")
	}
	if snap.StepRuntime["else.path"].Status != surface.StepSkipped {
		t.Fatalf("else.path status = %v, want StepSkipped", snap.StepRuntime["else.path"].Status)
	}
	if snap.StepRuntime["default.path"].Status != surface.StepSkipped {
		t.Fatalf("default.path status = %v, want StepSkipped", snap.StepRuntime["default.path"].Status)
	}
}

func TestCoerceHelpers(t *testing.T) {
	if got := asInt(nil); got != 0 {
		t.Fatalf("asInt(nil) = %d", got)
	}
	if got := asInt(float64(42)); got != 42 {
		t.Fatalf("asInt(float64(42)) = %d", got)
	}
	if got := asInt(42); got != 42 {
		t.Fatalf("asInt(42) = %d", got)
	}
	if got := asInt(int64(99)); got != 99 {
		t.Fatalf("asInt(int64(99)) = %d", got)
	}
	if got := asInt("not-a-number"); got != 0 {
		t.Fatalf("asInt(string) = %d", got)
	}

	if got := asStringSlice(nil); got != nil {
		t.Fatalf("asStringSlice(nil) = %v", got)
	}
	if got := asStringSlice([]string{"a", "b"}); len(got) != 2 || got[0] != "a" {
		t.Fatalf("asStringSlice([]string) = %v", got)
	}
	if got := asStringSlice([]any{"x", "y"}); len(got) != 2 || got[0] != "x" {
		t.Fatalf("asStringSlice([]any) = %v", got)
	}
	if got := asStringSlice("scalar"); got != nil {
		t.Fatalf("asStringSlice(string) = %v", got)
	}

	if got := asString(nil); got != "" {
		t.Fatalf("asString(nil) = %q", got)
	}
	if got := asString("hello"); got != "hello" {
		t.Fatalf("asString = %q", got)
	}
	if got := asString(float64(42)); got != "" {
		t.Fatalf("asString(float64) = %q", got)
	}
}
