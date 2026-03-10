package tui

import (
	"fmt"
	"time"
)

type StatusBar struct {
	width int
	now   time.Time
	state RuntimeState
}

func NewStatusBar() StatusBar {
	return StatusBar{now: time.Now()}
}

func (b *StatusBar) SetWidth(width int) {
	b.width = width
}

func (b *StatusBar) SetNow(now time.Time) {
	b.now = now
}

func (b *StatusBar) SetState(state RuntimeState) {
	b.state = state
}

func (b StatusBar) View() string {
	dot := statusOfflineStyle.Render("●")
	status := "offline"
	if b.state.Online {
		dot = statusOnlineStyle.Render("●")
		status = "online"
	}
	uptime := b.state.Uptime
	if !b.now.IsZero() && uptime == 0 {
		uptime = time.Since(b.now.Add(-uptime)).Round(time.Second)
	}
	text := fmt.Sprintf("%s %s  pid:%d  tenant:%s  seq:%d  %s", dot, status, b.state.PID, emptyFallback(b.state.TenantID, "default"), b.state.LastSeq, uptime.Round(time.Second))
	return statusBarStyle.Width(b.width).Render(text)
}
