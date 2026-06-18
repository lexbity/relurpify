package euclotui

import (
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// stepEvent builds an euclo step telemetry event as the broadcast sink would
// deliver it (metadata is map[string]any, ints arrive as the producer set them).
func stepEvent(typ telemetry.EventType, stepID string, seq uint64, success bool) telemetry.Event {
	return telemetry.Event{
		Type:   typ,
		TaskID: "task-1",
		Seq:    seq,
		Metadata: map[string]any{
			"step_id": stepID,
			"index":   0,
			"total":   1,
			"success": success,
		},
	}
}

// TestEucloSurfaceApplyExecEvent_PopulatesRouter proves the production consumer
// path (EucloSurface.ApplyExecEvent → long-lived applier → router) projects
// live execution telemetry into the recipe stepper.
func TestEucloSurfaceApplyExecEvent_PopulatesRouter(t *testing.T) {
	s := NewSurface().(*EucloSurface)

	s.ApplyExecEvent(stepEvent("euclo.step.started", "s1", 1, false))
	s.ApplyExecEvent(stepEvent("euclo.step.completed", "s1", 2, true))

	snap := s.router.Snapshot()
	if snap.Macro < surface.MacroExecute {
		t.Fatalf("macro = %v, want >= MacroExecute", snap.Macro)
	}
	rt, ok := snap.StepRuntime["s1"]
	if !ok {
		t.Fatal("step s1 not projected into router")
	}
	if rt.Status != surface.StepDone {
		t.Fatalf("step s1 status = %v, want StepDone", rt.Status)
	}
}

// TestEucloSurfaceApplyExecEvent_DedupsStaleSeq proves the seq guard survives
// across calls (regression test for the bug where a per-event applier reset its
// high-water map, making dedup a no-op). A stale duplicate of the earlier
// step.started MUST NOT revert the step from done back to active.
func TestEucloSurfaceApplyExecEvent_DedupsStaleSeq(t *testing.T) {
	s := NewSurface().(*EucloSurface)

	s.ApplyExecEvent(stepEvent("euclo.step.started", "s1", 1, false))   // s1 active
	s.ApplyExecEvent(stepEvent("euclo.step.completed", "s1", 2, true))  // s1 done
	s.ApplyExecEvent(stepEvent("euclo.step.started", "s1", 1, false))   // stale dup → must be dropped

	snap := s.router.Snapshot()
	rt, ok := snap.StepRuntime["s1"]
	if !ok {
		t.Fatal("step s1 not projected into router")
	}
	if rt.Status != surface.StepDone {
		t.Fatalf("step s1 status = %v after stale duplicate, want StepDone "+
			"(a fresh-per-event applier would have re-applied the stale start)", rt.Status)
	}
}
