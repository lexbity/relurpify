package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
)

// History and conversation commands over the durable journal (P02/P12).
// Playback is read-only: opening history performs no execution, acquires no
// model lease, and never resumes a run implicitly.

// historyPort resolves the read-only journal port from the model, when the
// live runtime provides one.
func historyPort(m *RootModel) HistoryPort {
	if m == nil || m.runtime == nil {
		return nil
	}
	if hp, ok := m.runtime.(HistoryPort); ok {
		return hp
	}
	return nil
}

func rootHandleRuns(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	port := historyPort(m)
	if port == nil {
		m.addSystemMessage("Run history is unavailable: no durable journal in this session.")
		return m, nil
	}
	runs, err := port.ListRuns(context.Background())
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Run history failed: %v", err))
		return m, nil
	}
	if len(runs) == 0 {
		m.addSystemMessage("No runs recorded in this workspace journal yet.")
		return m, nil
	}
	var b strings.Builder
	b.WriteString("Recorded runs (newest first):\n")
	for i, run := range runs {
		if i >= 20 {
			fmt.Fprintf(&b, "  … and %d more (use /history <run-id>)\n", len(runs)-i)
			break
		}
		state := run.State
		if run.Live {
			state += " · live"
		}
		line := fmt.Sprintf("  %s  %s  cursor=%d  started %s", run.RunID, state, run.Cursor, run.StartedAt.Format(time.RFC3339))
		if run.Reason != "" {
			line += "  — " + run.Reason
		}
		b.WriteString(line + "\n")
	}
	m.addSystemMessage(b.String())
	return m, nil
}

func rootHandleHistory(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	port := historyPort(m)
	if port == nil {
		m.addSystemMessage("Run history is unavailable: no durable journal in this session.")
		return m, nil
	}
	runID := ""
	if len(args) > 0 {
		runID = strings.TrimSpace(args[0])
	}
	if runID == "" {
		return rootHandleRuns(m, nil)
	}
	events, err := port.ReadRunEvents(context.Background(), runID, 0, 100)
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("History for %s failed: %v", runID, err))
		return m, nil
	}
	snap, _, snapErr := snapshotRun(m, runID)
	var b strings.Builder
	if snapErr == nil {
		fmt.Fprintf(&b, "Run %s: state=%s cursor=%d started=%s", runID, snap.State, snap.Cursor, snap.StartedAt.Format(time.RFC3339))
		if snap.CompletedAt != nil {
			fmt.Fprintf(&b, " completed=%s", snap.CompletedAt.Format(time.RFC3339))
		}
		if snap.Reason != "" {
			fmt.Fprintf(&b, "\n  reason: %s", snap.Reason)
		}
		b.WriteString("\n")
	} else {
		fmt.Fprintf(&b, "Run %s: committed snapshot unavailable (%v)\n", runID, snapErr)
	}
	if len(events) == 0 {
		b.WriteString("  no committed events retained\n")
	} else {
		b.WriteString("Committed journal events:\n")
		for _, ev := range events {
			fmt.Fprintf(&b, "  [%d] %s → %s  (%s)\n", ev.Cursor, ev.From, ev.To, ev.CreatedAt.Format("15:04:05"))
			if ev.Reason != "" {
				fmt.Fprintf(&b, "      %s\n", ev.Reason)
			}
		}
	}
	m.addSystemMessage(b.String())
	return m, nil
}

// snapshotRun reads one run's committed state through the history port.
func snapshotRun(m *RootModel, runID string) (runtimesvc.RunSummary, bool, error) {
	port := historyPort(m)
	if port == nil {
		return runtimesvc.RunSummary{}, false, fmt.Errorf("history unavailable")
	}
	return port.RunSnapshot(context.Background(), runID)
}

func rootHandleConversation(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	action := ""
	if len(args) > 0 {
		action = strings.ToLower(strings.TrimSpace(args[0]))
	}
	if m.chat == nil {
		m.addSystemMessage("No conversation canvas is active.")
		return m, nil
	}
	switch action {
	case "clear":
		// A new empty future context: committed transcript and run history
		// remain available through /history and the session record.
		m.chat.ClearMessages()
		m.chat.ResetUndoHistory()
		m.addSystemMessage("Conversation cleared. Committed run history remains available via /runs and /history.")
		return m, nil
	case "undo":
		if m.chat.Undo() {
			m.addSystemMessage("Conversation restored to the previous revision (context only; tools are never undone).")
		} else {
			m.addSystemMessage("Nothing to undo.")
		}
		return m, nil
	case "redo":
		if m.chat.Redo() {
			m.addSystemMessage("Conversation moved to the next revision (context only).")
		} else {
			m.addSystemMessage("Nothing to redo.")
		}
		return m, nil
	default:
		m.addSystemMessage("Usage: /conversation clear|undo|redo")
		return m, nil
	}
}

// registerJournalCommands registers the durable-run and conversation commands.
func registerJournalCommands(r *CommandRegistry) {
	r.Register(Command{Name: "runs", Description: "List runs recorded in the durable journal", Usage: "/runs", Handler: rootHandleRuns})
	r.Register(Command{Name: "history", Description: "Replay one run's committed journal (read-only)", Usage: "/history [run-id]", Handler: rootHandleHistory})
	r.Register(Command{Name: "conversation", Description: "Conversation context revisions", Usage: "/conversation clear|undo|redo", Handler: rootHandleConversation, TabFilter: []TabID{TabChat}})
}
