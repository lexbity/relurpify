package tui

import (
	"context"
	"fmt"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// chatPaneOf returns the underlying *ChatPane from a ChatPaner interface value.
// Used by tests that need to read or set internal ChatPane fields.
func chatPaneOf(c ChatPaner) *ChatPane { return c.(*ChatPane) }

// compactAdapter is a RuntimeAdapter stub that records ExecuteInstruction calls
// and returns a preset result or error.
type compactAdapter struct {
	recordingAdapter // embed for all the boilerplate stubs

	execResult *core.Result
	execErr    error
	lastPrompt string
	lastTask   core.TaskType
}

func (a *compactAdapter) ExecuteInstruction(_ context.Context, prompt string, taskType core.TaskType, _ map[string]any) (*core.Result, error) {
	a.lastPrompt = prompt
	a.lastTask = taskType
	return a.execResult, a.execErr
}

// --- helpers ---

// newCompactModel returns a RootModel with a populated chat pane containing n messages.
func newCompactModel(t *testing.T, rt RuntimeAdapter, n int) *RootModel {
	t.Helper()
	m := newRootModel(rt)
	mp := &m
	for i := 0; i < n; i++ {
		role := RoleUser
		if i%2 == 1 {
			role = RoleAgent
		}
		mp.chat.AppendMessage(Message{
			ID:        fmt.Sprintf("msg-%d", i),
			Role:      role,
			Timestamp: time.Now(),
			Content:   MessageContent{Text: fmt.Sprintf("message body %d", i)},
		})
	}
	return mp
}

// makeCompactRunState creates a RunState with streamed text pre-loaded for testing
// handleStreamComplete compact path.
func makeCompactRunState(id, text string) *RunState {
	builder := NewMessageBuilder(id)
	builder.AddToken(StreamTokenMsg{RunID: id, TokenType: TokenText, Token: text})
	return &RunState{
		ID:      id,
		Builder: builder,
		Ch:      make(chan tea.Msg, 4),
	}
}

// --- tests: rootHandleCompact guards ---

// TestCompactBlockedDuringActiveRun verifies /compact is a no-op while a run is active.
func TestCompactBlockedDuringActiveRun(t *testing.T) {
	rt := &compactAdapter{}
	m := newCompactModel(t, rt, 3)
	chatPaneOf(m.chat).runStates["fake-run-id"] = &RunState{ID: "fake-run-id"}

	m2, cmd := rootHandleCompact(m, nil)
	require.Nil(t, cmd)
	msgs := m2.chat.Messages()
	require.Equal(t, 3+1, len(msgs)) // 3 history + 1 system
	require.Equal(t, RoleSystem, msgs[len(msgs)-1].Role)
	require.Contains(t, msgs[len(msgs)-1].Content.Text, "cannot compact")
}

// TestCompactNoMessagesIsNoop verifies /compact on an empty feed does nothing.
func TestCompactNoMessagesIsNoop(t *testing.T) {
	rt := &compactAdapter{}
	m := newCompactModel(t, rt, 0)

	m2, cmd := rootHandleCompact(m, nil)
	require.Nil(t, cmd)
	msgs := m2.chat.Messages()
	require.Equal(t, 1, len(msgs))
	require.Contains(t, msgs[0].Content.Text, "nothing to compact")
}

// TestCompactSnapshotsUndoStack verifies the feed is pushed onto the undo stack
// and compactRunID is set before the run starts.
func TestCompactSnapshotsUndoStack(t *testing.T) {
	rt := &compactAdapter{}
	m := newCompactModel(t, rt, 4)

	cp := chatPaneOf(m.chat)
	undoBefore := len(cp.undoStack)
	_, cmd := rootHandleCompact(m, nil)
	require.NotNil(t, cmd)
	require.Equal(t, undoBefore+1, len(cp.undoStack), "undo stack should grow by 1")
	require.NotEmpty(t, cp.compactRunID, "compactRunID should be set")
	require.Equal(t, 4, cp.compactMsgCount)
}

// --- tests: streaming completion path ---

// TestCompactStreamCompleteEmitsCompactResult verifies that handleStreamComplete
// for a compact run emits compactResultMsg (not appending to the feed).
func TestCompactStreamCompleteEmitsCompactResult(t *testing.T) {
	rt := &compactAdapter{}
	m := newCompactModel(t, rt, 3)

	const runID = "compact-run-1"
	chatPaneOf(m.chat).runStates[runID] = makeCompactRunState(runID, "the generated summary")
	m.chat.SetCompactRunID(runID, 3)

	feedLenBefore := len(m.chat.Messages())

	p, cmd := m.chat.Update(StreamCompleteMsg{RunID: runID})
	require.NotNil(t, cmd)
	require.Empty(t, chatPaneOf(p).compactRunID, "compactRunID should be cleared")

	// Feed must NOT have a new message appended.
	require.Equal(t, feedLenBefore, len(p.Messages()))

	// The returned Cmd must yield a compactResultMsg with the streamed text.
	msg := cmd()
	result, ok := msg.(compactResultMsg)
	require.True(t, ok)
	require.NoError(t, result.err)
	require.Equal(t, "the generated summary", result.summary)
	require.Equal(t, 3, result.originalCount)
}

// TestCompactStreamErrorEmitsCompactResultError verifies that a stream error for a
// compact run emits compactResultMsg{err: ...} so model.go rolls back the undo snapshot.
func TestCompactStreamErrorEmitsCompactResultError(t *testing.T) {
	rt := &compactAdapter{}
	m := newCompactModel(t, rt, 2)

	const runID = "compact-run-err"
	chatPaneOf(m.chat).runStates[runID] = &RunState{ID: runID, Ch: make(chan tea.Msg, 4)}
	m.chat.SetCompactRunID(runID, 2)

	p, cmd := m.chat.Update(StreamErrorMsg{RunID: runID, Error: fmt.Errorf("model crashed")})
	require.NotNil(t, cmd)
	require.Empty(t, chatPaneOf(p).compactRunID)

	msg := cmd()
	result, ok := msg.(compactResultMsg)
	require.True(t, ok)
	require.Error(t, result.err)
	require.Contains(t, result.err.Error(), "model crashed")
	require.Equal(t, 2, result.originalCount)
}

// TestCompactStreamCompleteEmptyTextFallsBack verifies extractCompactSummary is
// used when the builder text is empty (e.g. model returned no streaming tokens).
func TestCompactStreamCompleteEmptyTextFallsBack(t *testing.T) {
	rt := &compactAdapter{}
	m := newCompactModel(t, rt, 1)

	const runID = "compact-run-empty"
	// Builder has no text tokens — only a result with data.
	builder := NewMessageBuilder(runID)
	chatPaneOf(m.chat).runStates[runID] = &RunState{
		ID:      runID,
		Builder: builder,
		Ch:      make(chan tea.Msg, 4),
	}
	m.chat.SetCompactRunID(runID, 1)

	_, cmd := m.chat.Update(StreamCompleteMsg{
		RunID:  runID,
		Result: &core.Result{Data: map[string]any{"final_output": "fallback summary"}},
	})
	require.NotNil(t, cmd)
	msg := cmd()
	result := msg.(compactResultMsg)
	require.NoError(t, result.err)
	require.Equal(t, "fallback summary", result.summary)
}

// TestNonCompactStreamCompleteUnchanged verifies that a regular (non-compact) run
// still appends to the feed and is not intercepted.
func TestNonCompactStreamCompleteUnchanged(t *testing.T) {
	rt := &compactAdapter{}
	m := newCompactModel(t, rt, 1)

	const runID = "normal-run"
	chatPaneOf(m.chat).runStates[runID] = makeCompactRunState(runID, "normal agent response")
	// compactRunID is NOT set — this is a normal run.

	feedLenBefore := len(m.chat.Messages())
	p, _ := m.chat.Update(StreamCompleteMsg{RunID: runID})

	// Feed should grow by 1 (the agent message).
	require.Equal(t, feedLenBefore+1, len(p.Messages()))
}

// --- tests: model.go compactResultMsg handler ---

// TestCompactResultMsgReplacesFeed verifies the model Update() handler replaces
// the feed with the summary and system notification on success.
func TestCompactResultMsgReplacesFeed(t *testing.T) {
	rt := &compactAdapter{}
	m := newCompactModel(t, rt, 5)

	m.chat.PushUndoSnapshot(m.chat.Messages())

	newM, _ := m.Update(compactResultMsg{summary: "compact summary here", originalCount: 5})
	rootM := newM.(RootModel)

	msgs := rootM.chat.Messages()
	require.Equal(t, 2, len(msgs), "feed should have system msg + summary")
	require.Equal(t, RoleSystem, msgs[0].Role)
	require.Contains(t, msgs[0].Content.Text, "5 messages")
	require.Equal(t, RoleAgent, msgs[1].Role)
	require.Equal(t, "compact summary here", msgs[1].Content.Text)
}

// TestCompactResultMsgErrorRollsBackUndo verifies that on error the undo snapshot
// added by rootHandleCompact is removed.
func TestCompactResultMsgErrorRollsBackUndo(t *testing.T) {
	rt := &compactAdapter{}
	m := newCompactModel(t, rt, 3)

	m.chat.PushUndoSnapshot(m.chat.Messages())
	undoBefore := len(chatPaneOf(m.chat).undoStack)

	newM, _ := m.Update(compactResultMsg{err: fmt.Errorf("oops"), originalCount: 3})
	rootM := newM.(RootModel)

	require.Equal(t, undoBefore-1, len(chatPaneOf(rootM.chat).undoStack), "undo snapshot should be rolled back")
}

// --- tests: helpers ---

// TestBuildCompactPromptSkipsSystemMessages verifies system messages are excluded.
func TestBuildCompactPromptSkipsSystemMessages(t *testing.T) {
	msgs := []Message{
		{Role: RoleSystem, Content: MessageContent{Text: "system line"}},
		{Role: RoleUser, Content: MessageContent{Text: "user question"}},
		{Role: RoleAgent, Content: MessageContent{Text: "agent answer"}},
	}
	prompt := buildCompactPrompt(msgs)
	require.NotContains(t, prompt, "system line")
	require.Contains(t, prompt, "user question")
	require.Contains(t, prompt, "agent answer")
}

// TestBuildCompactPromptNewestFirst verifies that when the budget is exceeded,
// the most recent messages are kept rather than the oldest.
func TestBuildCompactPromptNewestFirst(t *testing.T) {
	// Build a long history that exceeds the budget.
	msgs := make([]Message, 20)
	for i := range msgs {
		msgs[i] = Message{
			Role:    RoleUser,
			Content: MessageContent{Text: fmt.Sprintf("msg-%d %s", i, string(make([]byte, 1000)))},
		}
	}
	// Last message should always be in the prompt.
	msgs[19].Content.Text = "the very latest message"

	prompt := buildCompactPrompt(msgs)
	require.Contains(t, prompt, "the very latest message", "most recent message must be included")
}

// TestExtractCompactSummary verifies key priority and empty-string handling.
func TestExtractCompactSummary(t *testing.T) {
	require.Equal(t, "final", extractCompactSummary(&core.Result{Data: map[string]any{"final_output": "final", "text": "text"}}))
	require.Equal(t, "text val", extractCompactSummary(&core.Result{Data: map[string]any{"text": "text val"}}))
	require.Equal(t, "plan val", extractCompactSummary(&core.Result{Data: map[string]any{"summary": "plan val"}}))
	require.Equal(t, "", extractCompactSummary(&core.Result{Data: map[string]any{"final_output": "  "}}))
	require.Equal(t, "", extractCompactSummary(nil))
}
