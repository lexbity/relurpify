package tui

import (
	"fmt"
	"strings"
	"time"
)

func sectionTitle(text string) string {
	return sectionTitleStyle.Render(text)
}

func kv(key, value string) string {
	return keyStyle.Render(key+":") + " " + value
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func gatewayExposure(bind string) string {
	if strings.HasPrefix(bind, ":") || strings.HasPrefix(bind, "127.0.0.1:") || strings.HasPrefix(bind, "localhost:") || strings.HasPrefix(bind, "[::1]:") {
		return "local-only"
	}
	return "network"
}

func timeUntil(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Until(t).Round(time.Second)
	if d < 0 {
		return "expired"
	}
	return d.String()
}

func matchesFilter(filter, value string) bool {
	if strings.TrimSpace(filter) == "" {
		return true
	}
	return strings.Contains(strings.ToLower(value), strings.ToLower(filter))
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func pairingBadge(count int) string {
	if count == 0 {
		return ""
	}
	return fmt.Sprintf("%d!", count)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func paneWidthFor(width int) int {
	panelWidth := (width - 6) / 2
	if panelWidth < 32 {
		return max(width-2, 20)
	}
	return panelWidth
}
