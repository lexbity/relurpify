package euclotui

import (
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
)

func TestChatPaneSidebarWidthCollapsesAndExpands(t *testing.T) {
	pane := NewChatPane(nil, &tui.AgentContext{}, &tui.Session{}, &tui.NotificationQueue{}, nil)

	if got := pane.sidebarWidth(89); got != 0 {
		t.Fatalf("sidebarWidth(89) = %d, want 0", got)
	}
	if got := pane.sidebarWidth(119); got != 28 {
		t.Fatalf("sidebarWidth(119) = %d, want 28", got)
	}
	if got := pane.sidebarWidth(120); got != 32 {
		t.Fatalf("sidebarWidth(120) = %d, want 32", got)
	}

	pane.ToggleSidebar()
	pane.SetSize(89, 20)
	view := pane.View()
	if view == "" {
		t.Fatal("expected warning view under 90 columns")
	}
	if pane.splitSidebarVisible() {
		t.Fatal("expected sidebar to collapse under 90 columns")
	}
	pane.SetSize(120, 20)
	if !pane.splitSidebarVisible() {
		t.Fatal("expected sidebar to expand at 120 columns")
	}
}

func TestChatPaneWorkspaceSelectionWritesEnvelope(t *testing.T) {
	pane := NewChatPane(nil, &tui.AgentContext{}, &tui.Session{ID: "session-1"}, &tui.NotificationQueue{}, nil)

	if err := pane.AddFileToSidebar("alpha.go"); err != nil {
		t.Fatalf("AddFileToSidebar: %v", err)
	}
	if pane.selectionEnv == nil {
		t.Fatal("expected selection envelope to be initialized")
	}
	files, ok := euclostate.GetUserSelectedFiles(pane.selectionEnv)
	if !ok {
		t.Fatal("expected user_selected_files to be written")
	}
	if len(files) != 1 || files[0] != "alpha.go" {
		t.Fatalf("user_selected_files = %#v, want [alpha.go]", files)
	}

	pane.RemoveFileFromSidebar("alpha.go")
	files, ok = euclostate.GetUserSelectedFiles(pane.selectionEnv)
	if !ok {
		t.Fatal("expected user_selected_files after removal")
	}
	if len(files) != 0 {
		t.Fatalf("user_selected_files after removal = %#v, want empty", files)
	}
}

func TestChatPaneMilestoneFiltering(t *testing.T) {
	pane := NewChatPane(nil, &tui.AgentContext{}, &tui.Session{}, &tui.NotificationQueue{}, nil)

	pane.AppendMessage(tui.Message{
		Role: tui.RoleAgent,
		Content: tui.MessageContent{
			Text: "{\n  \"frame_id\": \"raw\"\n}",
		},
	})
	if got := pane.Messages(); len(got) != 0 {
		t.Fatalf("expected raw event noise to be filtered, got %#v", got)
	}

	pane.AppendMessage(tui.Message{
		Role: tui.RoleAgent,
		Content: tui.MessageContent{
			Text: "Candidates\n- inspect parser\n- update docs",
		},
	})
	got := pane.Messages()
	if len(got) != 1 {
		t.Fatalf("messages len = %d, want 1", len(got))
	}
	if got[0].Content.Text != "● Candidates" {
		t.Fatalf("milestone text = %q, want %q", got[0].Content.Text, "● Candidates")
	}

	pane.AppendMessage(tui.Message{Role: tui.RoleSystem, Content: tui.MessageContent{Text: "ready"}})
	got = pane.Messages()
	if len(got) != 2 {
		t.Fatalf("messages len after system = %d, want 2", len(got))
	}
}
