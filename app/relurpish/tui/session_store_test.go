package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionMetaRoundTripNewFields(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{root: dir}

	now := time.Now()
	rec := SessionRecord{
		SessionMeta: SessionMeta{
			ID:         "test-roundtrip",
			Agent:      "euclo",
			Workspace:  "/test",
			UpdatedAt:  now,
			WorkflowID: "wf-123",
			Mode:       "architect",
			HasBKC:     true,
		},
		Messages: []Message{
			{Role: RoleUser, Content: MessageContent{Text: "hello"}},
		},
	}

	if err := store.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load("test-roundtrip")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.WorkflowID != "wf-123" {
		t.Errorf("WorkflowID = %q, want wf-123", loaded.WorkflowID)
	}
	if loaded.Mode != "architect" {
		t.Errorf("Mode = %q, want architect", loaded.Mode)
	}
	if !loaded.HasBKC {
		t.Error("HasBKC should be true")
	}
}

func TestSessionMetaOldRecordWithoutWorkflowID(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{root: dir}

	// Write a record manually without the new fields (simulating an old file).
	id := "old-session"
	oldJSON := `{
		"id": "old-session",
		"agent": "euclo",
		"workspace": "/old"
	}`
	sessionDir := filepath.Join(dir, id)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "session.json"), []byte(oldJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load(id)
	if err != nil {
		t.Fatalf("Load old record: %v", err)
	}
	if loaded.WorkflowID != "" {
		t.Errorf("old record WorkflowID = %q, want empty", loaded.WorkflowID)
	}
	if loaded.HasBKC {
		t.Error("old record HasBKC should be false")
	}
}

func TestSessionListIncludesNewFields(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{root: dir}

	rec := SessionRecord{
		SessionMeta: SessionMeta{
			ID:         "list-test",
			Agent:      "euclo",
			WorkflowID: "wf-list",
			HasBKC:     true,
		},
	}
	if err := store.Save(rec); err != nil {
		t.Fatal(err)
	}

	metas, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(metas) == 0 {
		t.Fatal("expected at least one session")
	}
	found := false
	for _, m := range metas {
		if m.ID == "list-test" {
			found = true
			if m.WorkflowID != "wf-list" {
				t.Errorf("WorkflowID = %q, want wf-list", m.WorkflowID)
			}
			if !m.HasBKC {
				t.Error("HasBKC should be true")
			}
		}
	}
	if !found {
		t.Fatal("session not found in list")
	}
}

func TestSessionMetaWorkflowIDInResumeLabel(t *testing.T) {
	// Verify that sessions with WorkflowID show in the Welcome pane's
	// resume dropdown.
	store := NewSessionStore(t.TempDir())
	rec := SessionRecord{
		SessionMeta: SessionMeta{
			ID:         "resume-test",
			Agent:      "euclo",
			WorkflowID: "wf-resume",
		},
	}
	if err := store.Save(rec); err != nil {
		t.Fatal(err)
	}

	// Read it back via Load.
	loaded, err := store.Load("resume-test")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.WorkflowID != "wf-resume" {
		t.Errorf("WorkflowID = %q, want wf-resume", loaded.WorkflowID)
	}
}

func TestSessionMetaModeCaptured(t *testing.T) {
	dir := t.TempDir()
	store := &SessionStore{root: dir}

	rec := SessionRecord{
		SessionMeta: SessionMeta{
			ID:   "mode-test",
			Mode: "architect",
		},
	}
	if err := store.Save(rec); err != nil {
		t.Fatal(err)
	}

	metas, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range metas {
		if m.ID == "mode-test" && m.Mode != "architect" {
			t.Errorf("Mode = %q, want architect", m.Mode)
		}
	}
}
