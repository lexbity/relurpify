package tui

import (
	"testing"
)

// autosaveChatFake satisfies ChatPaner for autoSave, which only reads the
// transcript; every other method would be a nil-interface call.
type autosaveChatFake struct {
	ChatPaner
	msgs []Message
}

func (f *autosaveChatFake) Messages() []Message { return f.msgs }

// autosaveRuntimeFake satisfies RuntimeAdapter for autoSave, which only reads
// the active workflow ID.
type autosaveRuntimeFake struct {
	RuntimeAdapter
	active string
}

func (f *autosaveRuntimeFake) ActiveWorkflowID() string { return f.active }

func newAutoSaveModel(t *testing.T, store *SessionStore, rt RuntimeAdapter) RootModel {
	t.Helper()
	return RootModel{
		store:      store,
		chat:       &autosaveChatFake{msgs: []Message{{Role: RoleUser, Content: MessageContent{Text: "hello"}}}},
		sharedSess: &Session{ID: "sess-autosave", Workspace: t.TempDir()},
		runtime:    rt,
	}
}

func TestAutoSavePersistsTheFinishedRunWorkflowID(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	m := newAutoSaveModel(t, store, &autosaveRuntimeFake{active: "wf-stale"})

	m.autoSave("wf-run-42")

	rec, err := store.Load("sess-autosave")
	if err != nil {
		t.Fatalf("load autosaved record: %v", err)
	}
	if rec.WorkflowID != "wf-run-42" {
		t.Fatalf("record WorkflowID = %q, want the finished run's workflow ID", rec.WorkflowID)
	}
}

func TestAutoSavePreservesWorkflowIDWhenRunCarriesNone(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	m := newAutoSaveModel(t, store, nil) // no runtime: transcript-only session

	m.autoSave("wf-earlier")
	// A later run finished without a workflow ID (no lifecycle repository):
	// the resumable linkage must survive the rewrite.
	m.autoSave("")

	rec, err := store.Load("sess-autosave")
	if err != nil {
		t.Fatalf("load autosaved record: %v", err)
	}
	if rec.WorkflowID != "wf-earlier" {
		t.Fatalf("record WorkflowID = %q, want preserved wf-earlier", rec.WorkflowID)
	}
}

func TestAutoSaveFallsBackToActiveWorkflowID(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	m := newAutoSaveModel(t, store, &autosaveRuntimeFake{active: "wf-live"})

	m.autoSave("")

	rec, err := store.Load("sess-autosave")
	if err != nil {
		t.Fatalf("load autosaved record: %v", err)
	}
	if rec.WorkflowID != "wf-live" {
		t.Fatalf("record WorkflowID = %q, want the runtime's active workflow ID", rec.WorkflowID)
	}
}
