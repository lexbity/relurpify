package tui

import (
	_ "embed"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

//go:embed assets/ascii-art.txt
var mascotArt string

//go:embed assets/ascii-art-small.txt
var wordmarkArt string

// Logo renders the mascot (55×~20) or wordmark (61×5) depending on available
// space. Right-trims each line and drops trailing blank lines.
type Logo struct {
	width  int
	height int
}

// NewLogo creates a logo sized to the given pane dimensions.
func NewLogo(w, h int) *Logo {
	return &Logo{width: w, height: h}
}

// SetSize updates the available rendering space.
func (l *Logo) SetSize(w, h int) {
	l.width = w
	l.height = h
}

// View renders either the mascot or the wordmark, centred.
func (l *Logo) View() string {
	if l == nil || l.width < 1 || l.height < 1 {
		return ""
	}

	art := l.chooseArt()
	lines := prepareArt(art)
	return l.centre(lines)
}

// chooseArt selects mascot or wordmark based on available space.
func (l *Logo) chooseArt() string {
	if l.width >= 57 && l.height >= 22 {
		return mascotArt
	}
	return wordmarkArt
}

// prepareArt right-trims each line and drops trailing blank lines.
func prepareArt(art string) []string {
	raw := strings.Split(art, "\n")
	// Right-trim each line.
	trimmed := make([]string, 0, len(raw))
	for _, line := range raw {
		trimmed = append(trimmed, strings.TrimRight(line, " \t"))
	}
	// Drop trailing blank lines.
	end := len(trimmed)
	for end > 0 && strings.TrimSpace(trimmed[end-1]) == "" {
		end--
	}
	return trimmed[:end]
}

// centre returns the art centred horizontally and vertically within the
// available space.
func (l *Logo) centre(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	// Measure max line width.
	maxW := 0
	for _, line := range lines {
		w := lipgloss.Width(line)
		if w > maxW {
			maxW = w
		}
	}
	if maxW > l.width {
		maxW = l.width
	}
	// Pad lines to equal width (right padding normally, but we centre).
	padded := make([]string, len(lines))
	for i, line := range lines {
		padded[i] = lipgloss.NewStyle().Width(maxW).Render(line)
	}
	// Vertical centering.
	artH := len(padded)
	vPad := (l.height - artH) / 2
	if vPad < 0 {
		vPad = 0
	}
	result := make([]string, 0, vPad+len(padded)+vPad)
	for i := 0; i < vPad; i++ {
		result = append(result, "")
	}
	result = append(result, padded...)
	// Horizontal centering — rely on caller to Place or Join.
	return lipgloss.JoinVertical(lipgloss.Center, result...)
}
