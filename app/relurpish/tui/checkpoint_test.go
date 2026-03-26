package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCheckpointSaveAndList verifies checkpoint persistence and listing.
func TestCheckpointSaveAndList(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)

	rec := SessionRecord{
		SessionMeta: SessionMeta{
			Agent:     "test-agent",
			Workspace: tmpDir,
			Label:     "test-checkpoint",
		},
		Messages: []Message{
			{ID: "msg-1", Role: RoleUser, Content: MessageContent{Text: "hello"}},
		},
	}

	// Save checkpoint
	err := store.SaveCheckpoint(rec)
	require.NoError(t, err)

	// List checkpoints
	cps, err := store.ListCheckpoints()
	require.NoError(t, err)
	require.Len(t, cps, 1)
	require.Equal(t, "test-checkpoint", cps[0].Label)
	require.True(t, strings.HasPrefix(cps[0].ID, "ckpt-"))
}

// TestCheckpointIsolatedFromList verifies checkpoints don't appear in regular List().
func TestCheckpointIsolatedFromList(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)

	// Save a checkpoint
	cpRec := SessionRecord{
		SessionMeta: SessionMeta{
			Agent:     "agent",
			Workspace: tmpDir,
			Label:     "checkpoint1",
		},
	}
	err := store.SaveCheckpoint(cpRec)
	require.NoError(t, err)

	// List regular sessions should not include checkpoint
	sessions, err := store.List()
	require.NoError(t, err)
	for _, s := range sessions {
		require.False(t, strings.HasPrefix(s.ID, "ckpt-"), "checkpoint should not appear in List()")
	}

	// List checkpoints should find it
	cps, err := store.ListCheckpoints()
	require.NoError(t, err)
	require.Len(t, cps, 1)
}

// TestCheckpointMultiple verifies multiple checkpoints are stored and sorted by UpdatedAt.
func TestCheckpointMultiple(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)

	// Save first checkpoint
	rec1 := SessionRecord{
		SessionMeta: SessionMeta{
			Agent:     "agent",
			Workspace: tmpDir,
			Label:     "first",
		},
	}
	err := store.SaveCheckpoint(rec1)
	require.NoError(t, err)

	// Save second checkpoint
	rec2 := SessionRecord{
		SessionMeta: SessionMeta{
			Agent:     "agent",
			Workspace: tmpDir,
			Label:     "second",
		},
	}
	err = store.SaveCheckpoint(rec2)
	require.NoError(t, err)

	// List checkpoints
	cps, err := store.ListCheckpoints()
	require.NoError(t, err)
	require.Len(t, cps, 2)

	// Most recent should be first (sorted by UpdatedAt descending)
	require.Equal(t, "second", cps[0].Label)
	require.Equal(t, "first", cps[1].Label)
}

// TestCheckpointDefaultLabel verifies auto-generated label when none provided.
func TestCheckpointDefaultLabel(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)

	// Save checkpoint without label
	rec := SessionRecord{
		SessionMeta: SessionMeta{
			Agent:     "agent",
			Workspace: tmpDir,
			Label:     "", // no label provided
		},
	}
	err := store.SaveCheckpoint(rec)
	require.NoError(t, err)

	// Load and verify ID was generated
	cps, err := store.ListCheckpoints()
	require.NoError(t, err)
	require.Len(t, cps, 1)

	// Label should match the timestamp part of ID
	id := cps[0].ID
	require.True(t, strings.HasPrefix(id, "ckpt-"), "ID should start with ckpt-")
	// Extract the timestamp from ID (format: ckpt-<label>-<timestamp>)
	parts := strings.Split(id, "-")
	require.GreaterOrEqual(t, len(parts), 3, "ID should have format ckpt-label-timestamp")
}

// TestCheckpointCustomLabel verifies custom label is preserved.
func TestCheckpointCustomLabel(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)

	label := "before-refactor"
	rec := SessionRecord{
		SessionMeta: SessionMeta{
			Agent:     "agent",
			Workspace: tmpDir,
			Label:     label,
		},
	}
	err := store.SaveCheckpoint(rec)
	require.NoError(t, err)

	cps, err := store.ListCheckpoints()
	require.NoError(t, err)
	require.Len(t, cps, 1)
	require.Equal(t, label, cps[0].Label)
	require.Contains(t, cps[0].ID, label)
}

// TestCheckpointHandlerSavesWithLabel tests the /checkpoint command saves correctly.
func TestCheckpointHandlerSavesWithLabel(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := newMinimalCommitTestAdapter()
	m := newRootModel(adapter)
	m.store = NewSessionStore(tmpDir)
	m.sharedSess = &Session{
		ID:        "sess-123",
		Agent:     "test-agent",
		Workspace: tmpDir,
	}
	m.sharedCtx = &AgentContext{}

	// Execute checkpoint command with label
	updated, _ := rootHandleCheckpoint(&m, []string{"test-checkpoint"})

	// Verify checkpoint was saved
	cps, err := m.store.ListCheckpoints()
	require.NoError(t, err)
	require.Len(t, cps, 1)
	require.Equal(t, "test-checkpoint", cps[0].Label)

	// Verify system message was added
	messages := updated.chat.Messages()
	require.True(t, len(messages) > 0)
	lastMsg := messages[len(messages)-1]
	require.Equal(t, RoleSystem, lastMsg.Role)
	require.Contains(t, lastMsg.Content.Text, "checkpoint saved")
	require.Contains(t, lastMsg.Content.Text, "test-checkpoint")
}

// TestCheckpointHandlerAutoLabel tests the /checkpoint command with auto-generated label.
func TestCheckpointHandlerAutoLabel(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := newMinimalCommitTestAdapter()
	m := newRootModel(adapter)
	m.store = NewSessionStore(tmpDir)
	m.sharedSess = &Session{
		ID:        "sess-123",
		Agent:     "test-agent",
		Workspace: tmpDir,
	}
	m.sharedCtx = &AgentContext{}

	// Execute checkpoint command without label
	_, _ = rootHandleCheckpoint(&m, []string{})

	// Verify checkpoint was saved with auto label
	cps, err := m.store.ListCheckpoints()
	require.NoError(t, err)
	require.Len(t, cps, 1)

	// Label should be non-empty (auto-generated timestamp)
	require.NotEmpty(t, cps[0].Label)
}

// TestCheckpointHandlerError verifies error handling when store is unavailable.
func TestCheckpointHandlerError(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := newMinimalCommitTestAdapter()
	m := newRootModel(adapter)
	m.store = nil // no store
	m.sharedSess = &Session{
		ID:        "sess-123",
		Agent:     "test-agent",
		Workspace: tmpDir,
	}

	// Execute checkpoint command
	updated, _ := rootHandleCheckpoint(&m, []string{"test"})

	// Verify error message
	messages := updated.chat.Messages()
	require.True(t, len(messages) > 0)
	lastMsg := messages[len(messages)-1]
	require.Equal(t, RoleSystem, lastMsg.Role)
	require.Contains(t, lastMsg.Content.Text, "checkpoint unavailable")
}

// TestCheckpointWithMessages verifies checkpoint includes chat messages.
func TestCheckpointWithMessages(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)

	// Create a session record with messages
	messages := []Message{
		{ID: "msg-1", Role: RoleUser, Content: MessageContent{Text: "user message"}},
		{ID: "msg-2", Role: RoleAgent, Content: MessageContent{Text: "agent response"}},
	}
	rec := SessionRecord{
		SessionMeta: SessionMeta{
			Agent:     "agent",
			Workspace: tmpDir,
			Label:     "with-messages",
		},
		Messages: messages,
	}

	// Save checkpoint
	err := store.SaveCheckpoint(rec)
	require.NoError(t, err)

	// Load it back and verify messages are preserved
	cps, err := store.ListCheckpoints()
	require.NoError(t, err)
	require.Len(t, cps, 1)

	loaded, err := store.Load(cps[0].ID)
	require.NoError(t, err)
	require.Len(t, loaded.Messages, 2)
	require.Equal(t, "user message", loaded.Messages[0].Content.Text)
	require.Equal(t, "agent response", loaded.Messages[1].Content.Text)
}

// TestCheckpointDifferentLabels tests that labels with hyphens are preserved.
func TestCheckpointDifferentLabels(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewSessionStore(tmpDir)

	labels := []string{
		"before-refactor",
		"checkpoint-v1",
		"saved-state",
	}

	for _, label := range labels {
		rec := SessionRecord{
			SessionMeta: SessionMeta{
				Agent:     "agent",
				Workspace: tmpDir,
				Label:     label,
			},
		}
		err := store.SaveCheckpoint(rec)
		require.NoError(t, err)
	}

	// List all checkpoints
	cps, err := store.ListCheckpoints()
	require.NoError(t, err)
	require.Len(t, cps, len(labels))

	// Verify all labels are present (order will be newest first)
	foundLabels := make(map[string]bool)
	for _, cp := range cps {
		foundLabels[cp.Label] = true
	}
	for _, label := range labels {
		require.True(t, foundLabels[label], fmt.Sprintf("label %q should be found", label))
	}
}
