package euclotui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	tea "github.com/charmbracelet/bubbletea"
)

func TestDiffPaneGroupsByStepAndShowsVerificationAlerts(t *testing.T) {
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

	pane := NewDiffPane(router, "")
	pane.SetSize(140, 40)

	view := pane.View()
	for _, want := range []string{"Step: step-1", "Step: step-2", "fixtures/one.txt", "fixtures/two.txt", "Verification failed", "go test ./..."} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %s", want, view)
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

	pane := NewDiffPane(router, workspace)
	pane.SetSize(140, 40)

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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, string(got), want)
	}
}
