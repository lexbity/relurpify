package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TasksPane is now a minimal internal queue controller for sequential task execution.
// User-facing task inspection has been absorbed into the session/config surfaces.
type TasksPane struct {
	items  []TaskItem
	sel    int
	notifQ *NotificationQueue
	width  int
	height int
}

// NewTasksPane creates an empty task queue controller.
func NewTasksPane(_ RuntimeAdapter, notifQ *NotificationQueue) *TasksPane {
	return &TasksPane{notifQ: notifQ}
}

// SetSize stores the allocated size for optional rendering.
func (p *TasksPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// AddTask appends a task to the queue.
func (p *TasksPane) AddTask(item TaskItem) {
	if item.ID == "" {
		item.ID = generateID()
	}
	if item.Status == "" {
		item.Status = TaskPending
	}
	p.items = append(p.items, item)
}

// Items returns a snapshot of queued tasks for other panes to render.
func (p *TasksPane) Items() []TaskItem {
	if p == nil {
		return nil
	}
	out := make([]TaskItem, len(p.items))
	copy(out, p.items)
	return out
}

// MarkInProgress marks a task as running and stores its run ID.
func (p *TasksPane) MarkInProgress(taskID, runID string) {
	for i := range p.items {
		if p.items[i].ID == taskID {
			p.items[i].Status = TaskInProgress
			p.items[i].RunID = runID
			break
		}
	}
}

// MarkComplete marks a task done by run ID.
func (p *TasksPane) MarkComplete(runID string) {
	for i := range p.items {
		if p.items[i].RunID == runID {
			p.items[i].Status = TaskCompleted
			if p.notifQ != nil {
				p.notifQ.Push(NotificationItem{
					Kind:      NotifKindTaskDone,
					Msg:       fmt.Sprintf("Task done: %s", p.items[i].Description),
					CreatedAt: time.Now(),
				})
			}
			break
		}
	}
}

// NextPending returns the next pending task, if any.
func (p *TasksPane) NextPending() (TaskItem, bool) {
	for _, item := range p.items {
		if item.Status == TaskPending {
			return item, true
		}
	}
	return TaskItem{}, false
}

// Update only retains lightweight keyboard navigation for the queue list.
func (p *TasksPane) Update(msg tea.Msg) (*TasksPane, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if p.sel > 0 {
				p.sel--
			}
		case "down":
			if p.sel < len(p.items)-1 {
				p.sel++
			}
		}
	}
	return p, nil
}

// View renders the queued task list for any remaining internal callers.
func (p *TasksPane) View() string {
	if len(p.items) == 0 {
		return welcomeStyle.Render("No tasks queued.")
	}
	var b strings.Builder
	for i, item := range p.items {
		icon := "☐"
		style := taskPendingStyle
		switch item.Status {
		case TaskCompleted:
			icon = "✓"
			style = taskDoneStyle
		case TaskInProgress:
			icon = "●"
			style = taskRunningStyle
		}
		line := fmt.Sprintf("%s  %s", icon, style.Render(item.Description))
		if item.Agent != "" {
			line += dimStyle.Render(fmt.Sprintf("  [%s]", item.Agent))
		}
		if i == p.sel {
			line = panelItemActiveStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// HandleInputSubmit adds a new task from the input bar.
func (p *TasksPane) HandleInputSubmit(value string) tea.Cmd {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	p.AddTask(TaskItem{
		Description: strings.TrimSpace(value),
		Status:      TaskPending,
	})
	return nil
}
