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

	// Blob counts — populated when archaeo tab is active via BlobsUpdatedMsg.
	activeTab     TabID
	blobTensions  int
	blobPatterns  int
	blobLearning  int
	blobEmojiOn   bool
}

// NewTitleBar creates a TitleBar from session info.
func NewTitleBar(info SessionInfo) TitleBar {
	return TitleBar{
		agent:       info.Agent,
		model:       info.Model,
		workspace:   info.Workspace,
		blobEmojiOn: true,
	}
}

// SetActiveTab records which tab is currently active; affects right-hand badge rendering.
func (tb *TitleBar) SetActiveTab(id TabID) { tb.activeTab = id }

// SetBlobCounts updates the blob counts shown in the archaeo tab.
func (tb *TitleBar) SetBlobCounts(tensions, patterns, learning int) {
	tb.blobTensions = tensions
	tb.blobPatterns = patterns
	tb.blobLearning = learning
}

// SetBlobEmoji sets whether emoji or letter badges are used for blob counts.
func (tb *TitleBar) SetBlobEmoji(on bool) { tb.blobEmojiOn = on }

// Update stores the latest metrics (called after each run).
func (tb *TitleBar) Update(tokens int, dur time.Duration) {
	tb.tokens = tokens
	tb.duration = dur
}

// SetWidth propagates terminal width.
func (tb *TitleBar) SetWidth(w int) { tb.width = w }

// View renders the title bar.
func (tb TitleBar) View() string {
	tabLabel := "Chat"
	if tb.activeTab != "" {
		tabLabel = string(tb.activeTab)
	}
	left := fmt.Sprintf("%s · %s · %s", tabLabel, tb.agent, tb.model)

	// Right side: blob counts when on archaeo tab, else token/duration metrics.
	var right string
	if tb.activeTab == TabArchaeo {
		right = tb.blobCountBadge() + "  [ctrl+t]"
	} else {
		right = fmt.Sprintf("%s  %s  [ctrl+t]", formatTok(tb.tokens), fmtDuration(tb.duration))
	}

	pad := tb.width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 0 {
		pad = 0
	}
	return titleBarStyle.Width(tb.width).Render(left + strings.Repeat(" ", pad) + right)
}

// blobCountBadge renders the tension/pattern/learning count summary.
func (tb TitleBar) blobCountBadge() string {
	if tb.blobEmojiOn {
		return fmt.Sprintf("⚡%d  🧩%d  💡%d", tb.blobTensions, tb.blobPatterns, tb.blobLearning)
	}
	return fmt.Sprintf("T:%d P:%d L:%d", tb.blobTensions, tb.blobPatterns, tb.blobLearning)
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
