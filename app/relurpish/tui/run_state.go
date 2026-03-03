package tui

import (
	"context"
	"sync/atomic"
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

// UpdateTaskMsg allows external messages to update plan task status in-place.
type UpdateTaskMsg struct {
	TaskIndex int
	Status    TaskStatus
}

// listenToStream adapts a Go channel to a Bubble Tea Cmd.
func listenToStream(ch <-chan tea.Msg) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// sendRunMsg sends a message to the run channel without blocking.
// Dropped messages are counted atomically.
func sendRunMsg(run *RunState, msg tea.Msg) {
	if run == nil || run.Ch == nil {
		return
	}
	select {
	case run.Ch <- msg:
	default:
		atomic.AddInt64(&run.Dropped, 1)
	}
}

// sendRunFinal sends a message to the run channel, blocking in a goroutine
// if the channel is full (used for terminal messages that must not be lost).
func sendRunFinal(run *RunState, msg tea.Msg) {
	if run == nil || run.Ch == nil {
		return
	}
	select {
	case run.Ch <- msg:
	default:
		go func() {
			run.Ch <- msg
		}()
	}
}
