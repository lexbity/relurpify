package tui

import (
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// EditorExitMsg is emitted when a supervised editor process exits.
type EditorExitMsg struct {
	Path string
	PID  int
	Err  error
}

// EditorSupervisor tracks external editor processes opened from the TUI.
// It wraps tea.ExecProcess so that panes can open editors without managing
// child-process lifecycle directly. On editor exit, it emits EditorExitMsg
// which the model can use to re-run verification or restore terminal state.
type EditorSupervisor struct {
	mu         sync.Mutex
	activePID  int
	activePath string
}

var configuredEditor = "vi"

// SetEditor configures the editor command used by TUI helpers.
func SetEditor(editor string) {
	editor = strings.TrimSpace(editor)
	if editor == "" {
		editor = "vi"
	}
	configuredEditor = editor
}

// EditorPath returns the configured editor command.
func EditorPath() string {
	return configuredEditor
}

// OpenEditor spawns an external editor on the given file path and returns a
// tea.Cmd that the model or pane can return.  When the editor exits, an
// EditorExitMsg is sent back through Bubble Tea's event loop so the model can
// react (e.g. re-run verification, refresh state, or log the event).
func (s *EditorSupervisor) OpenEditor(path string) tea.Cmd {
	if s == nil {
		s = &EditorSupervisor{}
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	editor := configuredEditor
	if editor == "" {
		editor = "vi"
	}
	editorPath, err := exec.LookPath(editor)
	if err != nil {
		editorPath = editor
	}
	cmd := &exec.Cmd{
		Path: editorPath,
		Args: []string{editorPath, filepath.Clean(path)},
	}
	s.mu.Lock()
	s.activePath = path
	s.mu.Unlock()
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		pid := 0
		if cmd.Process != nil {
			pid = cmd.Process.Pid
		}
		if err != nil {
			s.mu.Lock()
			s.activePID = 0
			s.activePath = ""
			s.mu.Unlock()
			return EditorExitMsg{Path: path, PID: pid, Err: err}
		}
		s.mu.Lock()
		s.activePID = 0
		s.activePath = ""
		s.mu.Unlock()
		return EditorExitMsg{Path: path, PID: pid}
	})
}

// ActivePID returns the PID of the currently supervised editor, or 0 if none.
func (s *EditorSupervisor) ActivePID() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activePID
}

// ActivePath returns the file path being edited, or empty if no editor is open.
func (s *EditorSupervisor) ActivePath() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activePath
}

// IsActive returns true when an editor is currently being supervised.
func (s *EditorSupervisor) IsActive() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activePID != 0 || s.activePath != ""
}

// editorPath returns the editor executable path, falling back to "vi".
func editorPath() string {
	return configuredEditor
}

// editFileCmd returns a tea.Cmd that opens a file in the configured editor.
// This is a standalone helper for panes that do not need the full supervisor.
func editFileCmd(path string) tea.Cmd {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	editor := editorPath()
	if editor == "" {
		editor = "vi"
	}
	editorPath, err := exec.LookPath(editor)
	if err != nil {
		editorPath = editor
	}
	cmd := &exec.Cmd{
		Path: editorPath,
		Args: []string{editorPath, filepath.Clean(path)},
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		pid := 0
		if cmd.Process != nil {
			pid = cmd.Process.Pid
		}
		return EditorExitMsg{Path: path, PID: pid, Err: err}
	})
}

// filepathBase is a convenience wrapper used in editor-related log messages.
func editorBasename(path string) string {
	return filepath.Base(strings.TrimSpace(path))
}
