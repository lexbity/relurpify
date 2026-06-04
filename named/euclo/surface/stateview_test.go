package surface

import "testing"

func TestRenderClarificationRuntime(t *testing.T) {
	sv := StateView{
		ClarificationRuntimeLines: []string{
			"Task ID: task-1",
			"Clarification State Version: 3",
		},
	}

	want := "Clarification Runtime:\nTask ID: task-1\nClarification State Version: 3"
	if got := sv.RenderClarificationRuntime(); got != want {
		t.Errorf("RenderClarificationRuntime:\ngot  %q\nwant %q", got, want)
	}
}

func TestRenderClarificationRuntimeEmpty(t *testing.T) {
	sv := StateView{}
	if got := sv.RenderClarificationRuntime(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestRenderPlanGoalView(t *testing.T) {
	sv := StateView{
		PlanGoalViewLines: []string{
			"Task ID: task-1",
			"Task Goal: Review the codebase",
		},
	}

	want := "Clarification Plan View:\nTask ID: task-1\nTask Goal: Review the codebase"
	if got := sv.RenderPlanGoalView(); got != want {
		t.Errorf("RenderPlanGoalView:\ngot  %q\nwant %q", got, want)
	}
}

func TestRenderPlanGoalViewEmpty(t *testing.T) {
	sv := StateView{}
	if got := sv.RenderPlanGoalView(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestRenderPriorStepSummary(t *testing.T) {
	sv := StateView{
		PriorStepSummaryLines: []string{
			"Last Frame: review",
			"Previous Outcome: success",
		},
	}

	want := "Previous Step Summary:\nLast Frame: review\nPrevious Outcome: success"
	if got := sv.RenderPriorStepSummary(); got != want {
		t.Errorf("RenderPriorStepSummary:\ngot  %q\nwant %q", got, want)
	}
}

func TestRenderPriorStepSummaryEmpty(t *testing.T) {
	sv := StateView{}
	if got := sv.RenderPriorStepSummary(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestPromptReadsKeysNotEmpty(t *testing.T) {
	keys := PromptReadsKeys()
	if len(keys) == 0 {
		t.Fatal("PromptReadsKeys returned empty")
	}
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		if k == "" {
			t.Error("PromptReadsKeys contains empty string")
		}
		if seen[k] {
			t.Errorf("PromptReadsKeys contains duplicate %q", k)
		}
		seen[k] = true
	}
}
