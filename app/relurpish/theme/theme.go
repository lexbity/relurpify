package theme

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette holds the eight semantic colour slots. Every value is an adaptive
// colour with separate Light and Dark hex strings.
type Palette struct {
	Background lipgloss.AdaptiveColor
	Surface    lipgloss.AdaptiveColor
	Primary    lipgloss.AdaptiveColor
	Secondary  lipgloss.AdaptiveColor
	Success    lipgloss.AdaptiveColor
	Warning    lipgloss.AdaptiveColor
	Error      lipgloss.AdaptiveColor
	Dim        lipgloss.AdaptiveColor
}

// defaultPalette is the canonical relurpish palette. Light and Dark values
// match the former tui/styles.go colour globals.
var defaultPalette = Palette{
	Background: lipgloss.AdaptiveColor{Light: "#f4f4f5", Dark: "#1f1f23"},
	Surface:    lipgloss.AdaptiveColor{Light: "#d8d8dd", Dark: "#2b2f36"},
	Primary:    lipgloss.AdaptiveColor{Light: "#005f87", Dark: "#7fd7ff"},
	Secondary:  lipgloss.AdaptiveColor{Light: "#4f6d7a", Dark: "#8ad6c2"},
	Success:    lipgloss.AdaptiveColor{Light: "#2f7d32", Dark: "#87d75f"},
	Warning:    lipgloss.AdaptiveColor{Light: "#9a5f00", Dark: "#ffd75f"},
	Error:      lipgloss.AdaptiveColor{Light: "#b00020", Dark: "#ff8787"},
	Dim:        lipgloss.AdaptiveColor{Light: "#62666d", Dark: "#8d94a1"},
}

// Theme is the immutable semantic style source. It is safe for concurrent use.
// Use Default() to obtain the canonical theme and WithAccent() to produce a
// themed variant.
type Theme struct {
	pal    Palette
	accent *lipgloss.AdaptiveColor
}

// NotifKind classifies a notification badge.
type NotifKind int

const (
	NotifInfo    NotifKind = iota
	NotifHITL
	NotifSuccess
	NotifError
)

// Default returns the canonical default theme.
func Default() *Theme {
	return &Theme{pal: defaultPalette}
}

// WithAccent returns a clone of the theme with the accent colour overridden.
// The original theme is not mutated.
func (t *Theme) WithAccent(c lipgloss.AdaptiveColor) *Theme {
	clone := &Theme{pal: t.pal}
	accent := c
	clone.accent = &accent
	return clone
}

// Palette returns a copy of the theme's palette.
func (t *Theme) Palette() Palette {
	return t.pal
}

// accentOr returns the accent colour when set, otherwise fallback.
func (t *Theme) accentOr(fallback lipgloss.AdaptiveColor) lipgloss.AdaptiveColor {
	if t.accent != nil {
		return *t.accent
	}
	return fallback
}

// ── Text roles ──────────────────────────────────────────────────────────────

// Header renders emphasised heading text: bold, accent|primary.
func (t *Theme) Header() lipgloss.Style {
	if t == nil {
		t = Default()
	}
	return lipgloss.NewStyle().Bold(true).Foreground(t.accentOr(t.pal.Primary))
}

// Subhead renders a secondary heading: bold, secondary.
func (t *Theme) Subhead() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(t.pal.Secondary)
}

// Body renders default body text.
func (t *Theme) Body() lipgloss.Style {
	return lipgloss.NewStyle()
}

// Dim renders low-emphasis muted text.
func (t *Theme) Dim() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.pal.Dim)
}

// Detail renders dim italic text (hints, metadata).
func (t *Theme) Detail() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.pal.Dim).Italic(true)
}

// Success renders positive outcome text.
func (t *Theme) Success() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.pal.Success)
}

// Warning renders cautionary text.
func (t *Theme) Warning() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.pal.Warning)
}

// Error renders failure text.
func (t *Theme) Error() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.pal.Error)
}

// Active renders the focused/selected item: primary, bold.
func (t *Theme) Active() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.accentOr(t.pal.Primary)).Bold(true)
}

// Pending renders waiting/in-progress text (identical to Dim at this layer).
func (t *Theme) Pending() lipgloss.Style {
	return t.Dim()
}

// ── Container roles ─────────────────────────────────────────────────────────

// Panel renders a rounded-border container with dim border colour.
func (t *Theme) Panel() lipgloss.Style {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.pal.Dim).Padding(0, 1)
}

// Box renders a normal-border container with dim border colour.
func (t *Theme) Box() lipgloss.Style {
	return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(t.pal.Dim).Padding(0, 1)
}

// Bar renders a horizontal strip with surface background.
func (t *Theme) Bar() lipgloss.Style {
	return lipgloss.NewStyle().Background(t.pal.Surface).Padding(0, 1)
}

// Button renders a clickable or keyboard-activated pill.
func (t *Theme) Button(focused bool) lipgloss.Style {
	if focused {
		return lipgloss.NewStyle().Bold(true).Foreground(t.accentOr(t.pal.Primary)).Border(lipgloss.RoundedBorder()).Padding(0, 1)
	}
	return lipgloss.NewStyle().Foreground(t.pal.Dim).Border(lipgloss.RoundedBorder()).BorderForeground(t.pal.Dim).Padding(0, 1)
}

// Notif renders a notification badge for the given kind.
func (t *Theme) Notif(kind NotifKind) lipgloss.Style {
	switch kind {
	case NotifInfo:
		return lipgloss.NewStyle().Background(t.pal.Primary).Foreground(t.pal.Background).Padding(0, 1)
	case NotifHITL:
		return lipgloss.NewStyle().Background(t.pal.Warning).Foreground(t.pal.Background).Bold(true).Padding(0, 1)
	case NotifSuccess:
		return lipgloss.NewStyle().Background(t.pal.Success).Foreground(t.pal.Background).Padding(0, 1)
	case NotifError:
		return lipgloss.NewStyle().Background(t.pal.Error).Foreground(t.pal.Background).Padding(0, 1)
	default:
		return t.Dim().Padding(0, 1)
	}
}

// ── Animation support ───────────────────────────────────────────────────────

// Lerp linearly interpolates between two adaptive colours at progress p (0–1).
// Both the Light and Dark channels are interpolated independently. The result
// is itself an AdaptiveColor so that it correctly resolves at render time.
func (t *Theme) Lerp(from, to lipgloss.AdaptiveColor, p float64) lipgloss.AdaptiveColor {
	if p <= 0 {
		return from
	}
	if p >= 1 {
		return to
	}
	return lipgloss.AdaptiveColor{
		Light: lerpHex(from.Light, to.Light, p),
		Dark:  lerpHex(from.Dark, to.Dark, p),
	}
}

// lerpHex interpolates two hex colour strings at progress p (0–1).
func lerpHex(from, to string, p float64) string {
	r1, g1, b1 := parseHex(from)
	r2, g2, b2 := parseHex(to)
	r := int(clampF64(math.Round(r1 + (r2-r1)*p)))
	g := int(clampF64(math.Round(g1 + (g2-g1)*p)))
	b := int(clampF64(math.Round(b1 + (b2-b1)*p)))
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// parseHex extracts RGB from a hex colour string (#RRGGBB or #RGB).
func parseHex(s string) (float64, float64, float64) {
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return 0, 0, 0
	}
	r, _ := strconv.ParseUint(s[0:2], 16, 8)
	g, _ := strconv.ParseUint(s[2:4], 16, 8)
	b, _ := strconv.ParseUint(s[4:6], 16, 8)
	return float64(r), float64(g), float64(b)
}

func clampF64(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}
