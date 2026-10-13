package otelleg

import (
	"testing"

	"codeburg.org/lexbit/relurpify/telemetry"
)

func TestLifecycleFamilyLookup(t *testing.T) {
	for _, fam := range spanFamilies {
		for _, start := range fam.startTypes {
			got, ok := lifecycleFamilyFor(start)
			if !ok || got.name != fam.name {
				t.Fatalf("lifecycleFamilyFor(%q) = (%q,%v), want family %q", start, got.name, ok, fam.name)
			}
			if !isLifecycleStart(start) {
				t.Fatalf("isLifecycleStart(%q) = false, want true", start)
			}
		}
		for _, end := range fam.endTypes {
			got, ok := lifecycleFamilyFor(end)
			if !ok || got.name != fam.name {
				t.Fatalf("lifecycleFamilyFor(%q) = (%q,%v), want family %q", end, got.name, ok, fam.name)
			}
			if isLifecycleStart(end) {
				t.Fatalf("isLifecycleStart(%q) = true for an end type", end)
			}
		}
	}
}

func TestPointEventsAreNotLifecycle(t *testing.T) {
	for _, et := range []telemetry.EventType{
		telemetry.EventSchedulerJobCompleted,
		telemetry.EventCompilerWarning,
		telemetry.EventPolicyEvaluated,
		telemetry.EventType("command.mystery"),
	} {
		if isLifecycleType(et) {
			t.Fatalf("isLifecycleType(%q) = true for a point event", et)
		}
	}
}

// TestLifecycleTypesAreDeclared proves every start/end type in the family table
// is a real taxonomy constant, so the signal mapping cannot name a phantom type.
func TestLifecycleTypesAreDeclared(t *testing.T) {
	declared := map[string]bool{}
	for _, value := range declaredEventTypeValues(t) {
		declared[value] = true
	}
	for _, fam := range spanFamilies {
		for _, et := range append(append([]telemetry.EventType{}, fam.startTypes...), fam.endTypes...) {
			if !declared[string(et)] {
				t.Errorf("family %q references %q, which is not a declared event type", fam.name, et)
			}
		}
	}
}
