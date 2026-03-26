package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestChatPaneUndoWithSnapshot verifies undo restores previous snapshot.
func TestChatPaneUndoWithSnapshot(t *testing.T) {
	pane := NewChatPane(nil, nil, nil, nil)
	pane.SetSize(80, 20)

	// Add first message
	msg1 := Message{ID: "msg-1", Role: RoleUser, Content: MessageContent{Text: "First"}, Timestamp: time.Now()}
	pane.feed.AppendMessage(msg1)

	// Take snapshot before adding next message
	snapshot1 := make([]Message, len(pane.feed.Messages()))
	copy(snapshot1, pane.feed.Messages())
	pane.undoStack = append(pane.undoStack, snapshot1)

	// Add second message
	msg2 := Message{ID: "msg-2", Role: RoleAgent, Content: MessageContent{Text: "Second"}, Timestamp: time.Now()}
	pane.feed.AppendMessage(msg2)

	// Now we have 2 messages
	require.Len(t, pane.feed.Messages(), 2)

	// Undo should restore to first message only
	success := pane.Undo()
	require.True(t, success)
	require.Len(t, pane.feed.Messages(), 1)
	require.Equal(t, "msg-1", pane.feed.Messages()[0].ID)
}

// TestChatPaneRedoWithSnapshot verifies redo restores next snapshot.
func TestChatPaneRedoWithSnapshot(t *testing.T) {
	pane := NewChatPane(nil, nil, nil, nil)
	pane.SetSize(80, 20)

	// Add message
	msg1 := Message{ID: "msg-1", Role: RoleUser, Content: MessageContent{Text: "First"}, Timestamp: time.Now()}
	pane.feed.AppendMessage(msg1)

	// Undo does nothing with empty undo stack
	success := pane.Undo()
	require.False(t, success)

	// Manually set up redo stack
	msg2 := Message{ID: "msg-2", Role: RoleAgent, Content: MessageContent{Text: "Second"}, Timestamp: time.Now()}
	snapshot2 := []Message{msg1, msg2}
	pane.redoStack = append(pane.redoStack, snapshot2)

	// Redo should restore to 2 messages
	success = pane.Redo()
	require.True(t, success)
	require.Len(t, pane.feed.Messages(), 2)
	require.Equal(t, "msg-2", pane.feed.Messages()[1].ID)
}

// TestChatPaneUndoBlockedDuringActiveRun verifies undo is no-op when run is active.
func TestChatPaneUndoBlockedDuringActiveRun(t *testing.T) {
	pane := NewChatPane(nil, nil, nil, nil)
	pane.SetSize(80, 20)

	// Add message
	msg := Message{ID: "msg-1", Role: RoleUser, Content: MessageContent{Text: "Hello"}, Timestamp: time.Now()}
	pane.feed.AppendMessage(msg)

	// Add to undo stack
	snapshot := make([]Message, len(pane.feed.Messages()))
	copy(snapshot, pane.feed.Messages())
	pane.undoStack = append(pane.undoStack, snapshot)

	// Simulate active run
	pane.runStates["run-1"] = &RunState{}

	// Undo should fail
	success := pane.Undo()
	require.False(t, success)
	require.Len(t, pane.feed.Messages(), 1) // Message still there
}

// TestChatPaneRedoBlockedDuringActiveRun verifies redo is no-op when run is active.
func TestChatPaneRedoBlockedDuringActiveRun(t *testing.T) {
	pane := NewChatPane(nil, nil, nil, nil)
	pane.SetSize(80, 20)

	// Add message
	msg := Message{ID: "msg-1", Role: RoleUser, Content: MessageContent{Text: "Hello"}, Timestamp: time.Now()}
	pane.feed.AppendMessage(msg)

	// Add to redo stack
	snapshot := make([]Message, len(pane.feed.Messages()))
	copy(snapshot, pane.feed.Messages())
	pane.redoStack = append(pane.redoStack, snapshot)

	// Simulate active run
	pane.runStates["run-1"] = &RunState{}

	// Redo should fail
	success := pane.Redo()
	require.False(t, success)
	require.Len(t, pane.feed.Messages(), 1) // Message unchanged
}

// TestChatPaneUndoEmptyStack verifies undo fails on empty stack.
func TestChatPaneUndoEmptyStack(t *testing.T) {
	pane := NewChatPane(nil, nil, nil, nil)
	pane.SetSize(80, 20)

	// Add message
	msg := Message{ID: "msg-1", Role: RoleUser, Content: MessageContent{Text: "Hello"}, Timestamp: time.Now()}
	pane.feed.AppendMessage(msg)

	// Undo with empty stack should fail
	success := pane.Undo()
	require.False(t, success)
	require.Len(t, pane.feed.Messages(), 1) // Unchanged
}

// TestChatPaneRedoEmptyStack verifies redo fails on empty stack.
func TestChatPaneRedoEmptyStack(t *testing.T) {
	pane := NewChatPane(nil, nil, nil, nil)
	pane.SetSize(80, 20)

	// Add message
	msg := Message{ID: "msg-1", Role: RoleUser, Content: MessageContent{Text: "Hello"}, Timestamp: time.Now()}
	pane.feed.AppendMessage(msg)

	// Redo with empty stack should fail
	success := pane.Redo()
	require.False(t, success)
	require.Len(t, pane.feed.Messages(), 1) // Unchanged
}

// TestChatPaneUndoRedoRoundTrip verifies undo/redo pairs work correctly.
func TestChatPaneUndoRedoRoundTrip(t *testing.T) {
	pane := NewChatPane(nil, nil, nil, nil)
	pane.SetSize(80, 20)

	msg1 := Message{ID: "msg-1", Role: RoleUser, Content: MessageContent{Text: "First"}, Timestamp: time.Now()}
	msg2 := Message{ID: "msg-2", Role: RoleAgent, Content: MessageContent{Text: "Second"}, Timestamp: time.Now()}

	// Add first message
	pane.feed.AppendMessage(msg1)

	// Snapshot and add second
	snapshot1 := make([]Message, len(pane.feed.Messages()))
	copy(snapshot1, pane.feed.Messages())
	pane.undoStack = append(pane.undoStack, snapshot1)
	pane.feed.AppendMessage(msg2)

	// Snapshot and add third
	msg3 := Message{ID: "msg-3", Role: RoleUser, Content: MessageContent{Text: "Third"}, Timestamp: time.Now()}
	snapshot2 := make([]Message, len(pane.feed.Messages()))
	copy(snapshot2, pane.feed.Messages())
	pane.undoStack = append(pane.undoStack, snapshot2)
	pane.feed.AppendMessage(msg3)

	require.Len(t, pane.feed.Messages(), 3)

	// Undo once: back to 2 messages
	require.True(t, pane.Undo())
	require.Len(t, pane.feed.Messages(), 2)

	// Undo again: back to 1 message
	require.True(t, pane.Undo())
	require.Len(t, pane.feed.Messages(), 1)

	// Redo once: forward to 2 messages
	require.True(t, pane.Redo())
	require.Len(t, pane.feed.Messages(), 2)

	// Redo again: forward to 3 messages
	require.True(t, pane.Redo())
	require.Len(t, pane.feed.Messages(), 3)
}

// TestChatPaneUndoBuildsRedoStack verifies undo adds current state to redo stack.
func TestChatPaneUndoBuildsRedoStack(t *testing.T) {
	pane := NewChatPane(nil, nil, nil, nil)
	pane.SetSize(80, 20)

	msg1 := Message{ID: "msg-1", Role: RoleUser, Content: MessageContent{Text: "First"}, Timestamp: time.Now()}
	msg2 := Message{ID: "msg-2", Role: RoleAgent, Content: MessageContent{Text: "Second"}, Timestamp: time.Now()}

	pane.feed.AppendMessage(msg1)
	snapshot1 := make([]Message, len(pane.feed.Messages()))
	copy(snapshot1, pane.feed.Messages())
	pane.undoStack = append(pane.undoStack, snapshot1)

	pane.feed.AppendMessage(msg2)

	// Before undo: 2 messages, empty redo stack
	require.Len(t, pane.feed.Messages(), 2)
	require.Len(t, pane.redoStack, 0)

	// After undo: 1 message, redo stack has the 2-message state
	require.True(t, pane.Undo())
	require.Len(t, pane.feed.Messages(), 1)
	require.Len(t, pane.redoStack, 1)
	require.Equal(t, 2, len(pane.redoStack[0]))
}

// TestChatPaneSnapshotIndependence verifies snapshots are independent copies.
func TestChatPaneSnapshotIndependence(t *testing.T) {
	pane := NewChatPane(nil, nil, nil, nil)
	pane.SetSize(80, 20)

	msg1 := Message{ID: "msg-1", Role: RoleUser, Content: MessageContent{Text: "First"}, Timestamp: time.Now()}
	msg2 := Message{ID: "msg-2", Role: RoleAgent, Content: MessageContent{Text: "Second"}, Timestamp: time.Now()}

	pane.feed.AppendMessage(msg1)
	snapshot := make([]Message, len(pane.feed.Messages()))
	copy(snapshot, pane.feed.Messages())
	pane.undoStack = append(pane.undoStack, snapshot)

	pane.feed.AppendMessage(msg2)

	// Snapshot should still have only first message
	require.Len(t, pane.undoStack[0], 1)
	require.Equal(t, "msg-1", pane.undoStack[0][0].ID)

	// Feed should have both
	require.Len(t, pane.feed.Messages(), 2)
}
