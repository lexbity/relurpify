package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWelcomePaneSelectsWorkspaceFromRecentHistory(t *testing.T) {
	store := NewSessionStore(t.TempDir())
	now := time.Now()
	records := []SessionRecord{
		{SessionMeta: SessionMeta{ID: "a", Workspace: "/work/alpha", Agent: "euclo", Model: "m1", UpdatedAt: now.Add(-2 * time.Hour)}},
		{SessionMeta: SessionMeta{ID: "b", Workspace: "/work/beta", Agent: "euclo", Model: "m2", UpdatedAt: now.Add(-1 * time.Hour)}},
		{SessionMeta: SessionMeta{ID: "c", Workspace: "/work/beta", Agent: "none", Model: "m3", UpdatedAt: now}},
	}
	for _, rec := range records {
		if err := store.Save(rec); err != nil {
			t.Fatalf("save session %q: %v", rec.ID, err)
		}
	}

	pane := NewWelcomePane(&Session{}, store)
	items := pane.filteredWorkspaces()
	if got := len(items); got != 2 {
		t.Fatalf("workspace count = %d, want 2", got)
	}
	if got := items[0].Workspace; got != "/work/beta" {
		t.Fatalf("first workspace = %q, want /work/beta", got)
	}

	pane.selected = 1
	_, cmd := pane.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected enter to emit workspace selection")
	}
	msg := cmd()
	sel, ok := msg.(workspaceSelectedMsg)
	if !ok {
		t.Fatalf("message type = %T, want workspaceSelectedMsg", msg)
	}
	if sel.Workspace != "/work/alpha" {
		t.Fatalf("selected workspace = %q, want /work/alpha", sel.Workspace)
	}
}

func TestWelcomePaneFilterReducesWorkspaceList(t *testing.T) {
	store := NewSessionStore(t.TempDir())
	now := time.Now()
	records := []SessionRecord{
		{SessionMeta: SessionMeta{ID: "a", Workspace: "/work/alpha", UpdatedAt: now.Add(-2 * time.Hour)}},
		{SessionMeta: SessionMeta{ID: "b", Workspace: "/work/beta", UpdatedAt: now.Add(-1 * time.Hour)}},
	}
	for _, rec := range records {
		if err := store.Save(rec); err != nil {
			t.Fatalf("save session %q: %v", rec.ID, err)
		}
	}

	pane := NewWelcomePane(&Session{}, store)
	pane.SetFilter("bet")
	items := pane.filteredWorkspaces()
	if got := len(items); got != 1 {
		t.Fatalf("filtered workspace count = %d, want 1", got)
	}
	if got := items[0].Workspace; got != "/work/beta" {
		t.Fatalf("filtered workspace = %q, want /work/beta", got)
	}
}
