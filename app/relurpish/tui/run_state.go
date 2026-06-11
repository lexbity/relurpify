package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// RunState tracks a single in-flight execution.
type RunState struct {
	ID      string
	Prompt  string
	Started time.Time
	Builder *MessageBuilder
	Ch      chan tea.Msg
	Cancel  context.CancelFunc
	Dropped int64
}

// StreamDoneMsg signals that a streaming run completed and the host should
// perform its post-run bookkeeping.
type StreamDoneMsg struct{ RunID string }

type streamDoneMsg = StreamDoneMsg

// UpdateTaskMsg allows external messages to update plan task status in-place.
type UpdateTaskMsg struct {
	TaskIndex int
	Status    TaskStatus
}


