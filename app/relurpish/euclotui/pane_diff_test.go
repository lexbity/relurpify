package euclotui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	"codeburg.org/lexbit/relurpify/platform/fs"
)

func TestDiffPaneGroupsByFileAndShowsVerificationAlerts(t *testing.T) {
	router := NewEucloEventRouter()
	router.ApplyExecutionEvent(ExecutionEvent{
		StepID:  "step-1",
		Type:    reporting.EventTypeStepCompletedEuclo,
		Summary: "Create fixtures",
		PatchHunks: []PatchHunk{
			{
				File:    "fixtures/one.txt",
				Summary: "Create one",
				Body:    "one",
			},
		},
	})
	router.ApplyExecutionEvent(ExecutionEvent{
		StepID:  "step-2",
		Type:    reporting.EventTypeStepFailed,
		Summary: "Run verification",
		Output:  "go test ./...\nFAIL: TestParserRejects",
		PatchHunks: []PatchHunk{
			{
				File:    "fixtures/two.txt",
				Summary: "Update two",
				Body:    "two",
			},
		},
	})

	pane := NewDiffPane(router, "", nil)
	pane.SetSize(140, 40)

	view := pane.View()
	for _, want := range []string{"view by-file", "fixtures/one.txt", "fixtures/two.txt", "Verification failed", "go test ./..."} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %s", want, view)
		}
	}

	if cmd := pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}); cmd != nil {
		// mode toggle only
	}
	view = pane.View()
	for _, want := range []string{"view by-cause", "Step: step-1", "Step: step-2"} {
		if !strings.Contains(view, want) {
			t.Fatalf("by-cause view missing %q: %s", want, view)
		}
	}
}

func TestDiffPaneAppliesAndRevertsCausalChanges(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "a.txt"), "alpha-a\n")
	writeFile(t, filepath.Join(workspace, "b.txt"), "alpha-b\n")
	writeFile(t, filepath.Join(workspace, "c.txt"), "alpha-c\n")

	router := NewEucloEventRouter()
	router.ApplyExecutionEvent(ExecutionEvent{
		StepID:  "step-1",
		Type:    reporting.EventTypeStepCompletedEuclo,
		Summary: "Apply first group",
		PatchHunks: []PatchHunk{
			{File: "a.txt", Summary: "A", Body: "beta-a\n"},
			{File: "b.txt", Summary: "B", Body: "beta-b\n"},
		},
	})
	router.ApplyExecutionEvent(ExecutionEvent{
		StepID:  "step-2",
		Type:    reporting.EventTypeStepCompletedEuclo,
		Summary: "Apply second group",
		PatchHunks: []PatchHunk{
			{File: "c.txt", Summary: "C", Body: "beta-c\n"},
		},
	})

	pane := NewDiffPane(router, workspace, nil)
	pane.SetSize(140, 40)
	if cmd := pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}); cmd != nil {
		// switch to by-cause for step-scoped apply assertions
	}

	if cmd := pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}); cmd == nil {
		t.Fatal("expected apply-step command")
	}
	assertFileContents(t, filepath.Join(workspace, "a.txt"), "beta-a\n")
	assertFileContents(t, filepath.Join(workspace, "b.txt"), "beta-b\n")
	assertFileContents(t, filepath.Join(workspace, "c.txt"), "alpha-c\n")

	if cmd := pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}}); cmd == nil {
		t.Fatal("expected revert-all command")
	}
	assertFileContents(t, filepath.Join(workspace, "a.txt"), "alpha-a\n")
	assertFileContents(t, filepath.Join(workspace, "b.txt"), "alpha-b\n")
	assertFileContents(t, filepath.Join(workspace, "c.txt"), "alpha-c\n")

	if cmd := pane.Update(tea.KeyMsg{Type: tea.KeyDown}); cmd != nil {
		// movement only
	}
	if cmd := pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}); cmd == nil {
		t.Fatal("expected apply-file command")
	}
	assertFileContents(t, filepath.Join(workspace, "a.txt"), "beta-a\n")
	assertFileContents(t, filepath.Join(workspace, "b.txt"), "alpha-b\n")
	assertFileContents(t, filepath.Join(workspace, "c.txt"), "alpha-c\n")

	if cmd := pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}); cmd == nil {
		t.Fatal("expected apply-all command")
	}
	assertFileContents(t, filepath.Join(workspace, "a.txt"), "beta-a\n")
	assertFileContents(t, filepath.Join(workspace, "b.txt"), "beta-b\n")
	assertFileContents(t, filepath.Join(workspace, "c.txt"), "beta-c\n")
}

func TestDiffPaneShowsCheckpointAnchor(t *testing.T) {
	workspace := t.TempDir()
	store := tui.NewSessionStore(workspace)
	if err := store.SaveCheckpoint(tui.SessionRecord{
		SessionMeta: tui.SessionMeta{
			Workspace: workspace,
			Agent:     "guest",
			Label:     "anchor",
		},
	}); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}
	router := NewEucloEventRouter()
	router.ApplyExecutionEvent(ExecutionEvent{
		StepID:  "step-1",
		Type:    reporting.EventTypeStepCompletedEuclo,
		Summary: "Apply first group",
		PatchHunks: []PatchHunk{
			{File: "a.txt", Summary: "A", Body: "beta-a\n"},
		},
	})

	pane := NewDiffPane(router, workspace, nil)
	pane.SetSessionStore(store)
	pane.SetSize(140, 40)

	view := pane.View()
	if !strings.Contains(view, "checkpoint @ ckpt-anchor-") {
		t.Fatalf("view missing checkpoint anchor: %s", view)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), fs.PublicDirMode); err != nil { // public: test dir
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), fs.PublicFileMode); err != nil { // public: test fixture
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, string(got), want)
	}
}
