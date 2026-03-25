package rewoo

import "testing"

func TestCompactRewooToolResultsState(t *testing.T) {
	value := compactRewooToolResultsState([]RewooStepResult{
		{StepID: "a", Tool: "tool_a", Success: true},
		{StepID: "b", Tool: "tool_b", Success: false, Error: "failed"},
	})

	if got := value["step_count"]; got != 2 {
		t.Fatalf("expected step_count=2, got %#v", got)
	}
	if got := value["steps_ok"]; got != 1 {
		t.Fatalf("expected steps_ok=1, got %#v", got)
	}
	steps, ok := value["steps"].([]map[string]any)
	if !ok || len(steps) != 2 {
		t.Fatalf("expected compact steps, got %#v", value["steps"])
	}
	last, ok := value["last_step"].(map[string]any)
	if !ok || last["step_id"] != "b" {
		t.Fatalf("expected last_step for b, got %#v", value["last_step"])
	}
}
