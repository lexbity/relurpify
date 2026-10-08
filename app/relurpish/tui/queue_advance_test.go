package tui

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// queueChatPane is a minimal ChatPaner that records task-queue start calls.
type queueChatPane struct {
	ChatPaner
	started []string
}

func (q *queueChatPane) HasActiveRuns() bool { return false }

func (q *queueChatPane) StartRun(description string) (tea.Cmd, string) {
	q.started = append(q.started, description)
	return nil, "run-queue-test"
}

func (q *queueChatPane) Update(tea.Msg) (ChatPaner, tea.Cmd) { return q, nil }

func (q *queueChatPane) Messages() []Message { return nil }

// TestRunFinishedAdvancesTaskQueueAfterError: the task queue advances on
// RunFinishedMsg regardless of outcome — an errored run must not stall the
// queue (the defect this terminal message exists to retire).
func TestRunFinishedAdvancesTaskQueueAfterError(t *testing.T) {
	m := newRootModel(context.Background(), nil, NewDefaultSurfaceFactory())
	chat := &queueChatPane{}
	m.chat = chat
	m.tasks.AddTask(TaskItem{ID: "task-1", Description: "queued work", Status: TaskPending})

	next, _ := m.Update(RunFinishedMsg{
		RunID:   "run-previous",
		Outcome: RunFailed,
		Err:     errors.New("boom"),
	})
	m2, ok := next.(RootModel)
	if !ok {
		t.Fatalf("Update returned %T, want RootModel", next)
	}
	if len(chat.started) != 1 || chat.started[0] != "queued work" {
		t.Fatalf("queue did not advance after errored run; started=%v", chat.started)
	}
	items := m2.tasks.Items()
	if len(items) != 1 || items[0].Status != TaskInProgress || items[0].RunID != "run-queue-test" {
		t.Fatalf("next task not marked in progress: %+v", items)
	}
}
