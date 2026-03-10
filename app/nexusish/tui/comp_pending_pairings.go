package tui

import (
	"fmt"
	"strings"
)

type PendingPairingsView struct{}

func NewPendingPairingsView() PendingPairingsView {
	return PendingPairingsView{}
}

func (v PendingPairingsView) View(pairings []PendingPairingInfo, selected int, active bool) string {
	if len(pairings) == 0 {
		return mutedStyle.Render("No pending pairing requests.")
	}
	lines := make([]string, 0, len(pairings)+1)
	for idx, pairing := range pairings {
		line := fmt.Sprintf("%s  %s  exp %s", pairing.Code, pairing.DeviceID, timeUntil(pairing.ExpiresAt))
		if idx == selected && active {
			line = selectedLineStyle.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, mutedStyle.Render("[a] approve  [x] reject  [j/k] move"))
	return strings.Join(lines, "\n")
}
