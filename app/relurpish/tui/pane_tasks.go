package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TaskQueueCompleteMsg is emitted when a queued task run finishes.
type TaskQueueCompleteMsg struct{ RunID string }

// TasksPane shows queued tasks and their execution status.
type TasksPane struct {
	feed    *Feed
	items   []TaskItem
	sel     int
	notifQ  *NotificationQueue
	runtime RuntimeAdapter
	width   int
	height  int
}

// NewTasksPane creates an empty tasks pane.
func NewTasksPane(rt RuntimeAdapter, notifQ *NotificationQueue) *TasksPane {
	return &TasksPane{
		feed:    NewFeed(),
		notifQ:  notifQ,
		runtime: rt,
	}
}

// SetSize resizes the pane.
func (p *TasksPane) SetSize(w, h int) {
	p.width = w
	p.height = h
	p.feed.SetSize(w, max(1, h))
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
	p.rebuildFeed()
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
	p.rebuildFeed()
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
		}
	}
	p.rebuildFeed()
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

// Update handles tab-specific keys.
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
	case tea.MouseMsg:
		f, cmd := p.feed.Update(msg)
		p.feed = f
		return p, cmd
	}
	return p, nil
}

// View renders the tasks list.
func (p *TasksPane) View() string {
	if len(p.items) == 0 {
		return welcomeStyle.Render("No tasks queued. Type a task description and press Enter.")
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
	return b.String()
}

func (p *TasksPane) rebuildFeed() {
	// Tasks pane uses direct rendering, not a Feed of Messages.
	_ = p.feed
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
