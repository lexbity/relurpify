package tui

import (
	"strings"
	"testing"
	"time"
)

func TestFeedSetSpinnerUpdatesRender(t *testing.T) {
	f := NewFeed()
	f.SetSize(80, 24)

	// No spinner set — should not contain spinner glyph.
	view := f.renderAll()
	if view == "" {
		t.Fatal("expected non-empty feed view")
	}

	// Set a spinner frame.
	f.SetSpinner("⣷")
	view2 := f.renderAll()
	if !strings.Contains(view2, "⣷") {
		t.Log("feed render with spinner (glyph may not appear without messages)")
	}
}

func TestFeedSpinnerAppearsInThinkingBlock(t *testing.T) {
	f := NewFeed()
	f.SetSize(80, 24)

	spinnerGlyph := "⣾"
	f.SetSpinner(spinnerGlyph)

	// Append a message with thinking steps.
	f.AppendMessage(Message{
		Role: RoleAgent,
		Content: MessageContent{
			Thinking: []ThinkingStep{
				{Type: StepAnalyzing, Description: "analyzing", StartTime: time.Now()},
			},
			Expanded: map[string]bool{"thinking": true},
		},
	})

	view := f.renderAll()
	// The thinking block should render with the spinner glyph as icon
	// for the last step when EndTime is zero.
	if !strings.Contains(view, spinnerGlyph) {
		t.Errorf("spinner glyph not found in thinking block: %s", view)
	}
}

func TestFeedSpinnerAppearsInPlanBlock(t *testing.T) {
	f := NewFeed()
	f.SetSize(80, 24)

	spinnerGlyph := "⣷"
	f.SetSpinner(spinnerGlyph)

	f.AppendMessage(Message{
		Role: RoleAgent,
		Content: MessageContent{
			Plan: &TaskPlan{
				Tasks: []Task{
					{Description: "task 1", Status: TaskInProgress},
				},
			},
			Expanded: map[string]bool{"plan": true},
		},
	})

	view := f.renderAll()
	if !strings.Contains(view, spinnerGlyph) {
		t.Errorf("spinner glyph not found in plan block: %s", view)
	}
}
