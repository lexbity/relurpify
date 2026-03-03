package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// TitleBar renders the top-of-screen info bar.
type TitleBar struct {
	agent     string
	model     string
	workspace string
	tokens    int
	duration  time.Duration
	width     int
}

// NewTitleBar creates a TitleBar from session info.
func NewTitleBar(info SessionInfo) TitleBar {
	return TitleBar{
		agent:     info.Agent,
		model:     info.Model,
		workspace: info.Workspace,
	}
}

// Update stores the latest metrics (called after each run).
func (tb *TitleBar) Update(tokens int, dur time.Duration) {
	tb.tokens = tokens
	tb.duration = dur
}

// SetWidth propagates terminal width.
func (tb *TitleBar) SetWidth(w int) { tb.width = w }

// View renders the title bar.
func (tb TitleBar) View() string {
	left := fmt.Sprintf("Chat · %s · %s", tb.agent, tb.model)
	right := fmt.Sprintf("%s  %s  [ctrl+t]", formatTok(tb.tokens), fmtDuration(tb.duration))
	pad := tb.width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 0 {
		pad = 0
	}
	return titleBarStyle.Width(tb.width).Render(left + strings.Repeat(" ", pad) + right)
}

func formatTok(n int) string {
	if n == 0 {
		return "0 tok"
	}
	if n < 1000 {
		return fmt.Sprintf("%d tok", n)
	}
	return fmt.Sprintf("%.1fk tok", float64(n)/1000)
}

func fmtDuration(d time.Duration) string {
	if d == 0 {
		return "--"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}
