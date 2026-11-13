package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/telemetry"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestV2ActivityUsesTelemetryWithoutRetainingPayloads(t *testing.T) {
	secret := "do-not-retain-this-secret"
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	event := telemetry.Event{Type: telemetry.EventLLMPrompt, RunID: "run-real", TaskID: "task-real", NodeID: "node-real", Seq: 7, Timestamp: time.Now(), Message: secret, Metadata: map[string]any{
		"kind": "chat_with_tools", "model": "model-one", "messages_preview": secret, "prompt": secret, "args": cyclic, "usage": model.TokenUsage{PromptTokens: 3, CompletionTokens: 5, TotalTokens: 8}, "error": secret,
	}}
	p := activityCanvas{}
	p.apply([]telemetry.Event{event})
	event.Metadata["model"] = "mutated"
	p.inspector.key(tea.KeyMsg{Type: tea.KeyEnter})
	view := p.inspector.view(120, 60)
	for _, want := range []string{"run-real", "task-real", "node-real", "model-one", "Input tokens: 3", "Total tokens: 8", "not captured"} {
		require.Contains(t, view, want)
	}
	require.NotContains(t, fmt.Sprint(p.rows), secret)
	require.NotContains(t, view, "mutated")
	require.Len(t, p.rows, 1)
}

func TestV2ActivityRetainsReadingAndBounds(t *testing.T) {
	p := activityCanvas{}
	p.apply([]telemetry.Event{{Type: telemetry.EventCompilerCompleted, NodeID: "pin-me"}})
	p.inspector.key(tea.KeyMsg{Type: tea.KeyEnter})
	p.inspector.key(tea.KeyMsg{Type: tea.KeyDown})
	selected := p.inspector.selected
	for i := range 1000 {
		p.apply([]telemetry.Event{{Type: telemetry.EventNodeStart, NodeID: fmt.Sprint(i)}})
	}
	require.Len(t, p.rows, activityLimit)
	require.EqualValues(t, 489, p.evicted)
	require.Equal(t, selected, p.inspector.selected)
	require.True(t, p.inspector.detail)
	require.Equal(t, 1, p.inspector.offset)
	require.Len(t, p.inspector.document.Entries, activityLimit+1)
	require.Contains(t, p.inspector.view(120, 50), "pin-me")
	for _, size := range [][2]int{{80, 24}, {120, 40}, {240, 80}, {5, 3}} {
		for _, line := range strings.Split(p.inspector.view(size[0], size[1]), "\n") {
			require.LessOrEqual(t, ansi.StringWidth(line), size[0])
		}
		require.LessOrEqual(t, len(strings.Split(p.inspector.view(size[0], size[1]), "\n")), size[1])
	}
	p.inspector.key(tea.KeyMsg{Type: tea.KeyEsc})
	p.rebuild()
	require.Len(t, p.inspector.document.Entries, activityLimit)
}

func TestV2ActivityHostFilterAndDraftSurviveProgress(t *testing.T) {
	m, _ := newFocusTestModel()
	nextSized, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = nextSized.(RootModel)
	m.inputBar.SetValue("keep request")
	next, _ := m.Update(ExecEventBatchMsg{Events: []telemetry.Event{{Type: telemetry.EventCompilerStarted}, {Type: telemetry.EventLLMPrompt, Metadata: map[string]any{"model": "actual model"}}}})
	m = next.(RootModel)
	rootHandleActivity(&m, []string{"calls"})
	require.Equal(t, TabActivity, m.tabs.ActiveTab().ID)
	require.Len(t, m.activity.inspector.document.Entries, 1)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(RootModel)
	next, _ = m.Update(ExecEventBatchMsg{Events: []telemetry.Event{{Type: telemetry.EventNodeStart}}})
	m = next.(RootModel)
	require.Equal(t, "keep request", m.inputBar.Value())
	require.True(t, m.activity.inspector.detail)
	require.Contains(t, m.View(), "actual model")
	require.True(t, m.focus.State().InRegion1())
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(RootModel)
	require.False(t, m.activity.inspector.detail)
	require.True(t, m.focus.State().InRegion1())
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(RootModel)
	require.True(t, m.focus.State().InInput())
	require.Equal(t, "keep request", m.inputBar.Value())
}

func FuzzActivityProjection(f *testing.F) {
	f.Add("llm_prompt", "model", "\x1b[2Jsecret")
	f.Fuzz(func(t *testing.T, kind, name, payload string) {
		if len(kind)+len(name)+len(payload) > 100000 {
			t.Skip()
		}
		p := activityCanvas{}
		p.apply([]telemetry.Event{{Type: telemetry.EventType(kind), NodeID: name, Metadata: map[string]any{"model": payload, "prompt": payload}}})
		p.inspector.key(tea.KeyMsg{Type: tea.KeyEnter})
		for _, line := range strings.Split(p.inspector.view(80, 24), "\n") {
			require.LessOrEqual(t, ansi.StringWidth(line), 80)
		}
		require.NotContains(t, p.inspector.view(80, 24), "\x1b")
	})
}

func TestV2ActivityDistinguishesMissingEstimatedAndInvalidUsage(t *testing.T) {
	cases := []struct {
		usage       model.TokenUsage
		want        string
		hasCounters bool
	}{
		{model.TokenUsage{}, "Not established", false},
		{model.TokenUsage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}, "Reported counters", true},
		{model.TokenUsage{PromptTokens: 3, TotalTokens: 3, Estimated: true, EstimationMethod: "chars/4"}, "Estimated counters / chars/4", true},
		{model.TokenUsage{PromptTokens: -1}, "Invalid negative counters", false},
	}
	for _, tc := range cases {
		entry := activityInspection(telemetry.Event{Type: telemetry.EventLLMResponse, Metadata: map[string]any{"usage": tc.usage}}, 1, false)
		fields := fmt.Sprint(entry.Fields)
		require.Contains(t, fields, tc.want)
		require.Equal(t, tc.hasCounters, strings.Contains(fields, "Input tokens"))
	}
}
