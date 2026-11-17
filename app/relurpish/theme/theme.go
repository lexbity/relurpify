package theme

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"codeburg.org/lexbit/relurpify/named/euclo/surface"
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

// relurpifyContrastPalette is the default relurpish-contrast theme, aligned with
// the V2 keyframe scheme (devdocs/relurpish-keyframes/v2): dark charcoal/navy
// canvas, restrained surfaces, muted teal selection/activity, and higher-contrast
// readable text/status hues.
var relurpifyContrastPalette = Palette{ //nolint:gochecknoglobals // immutable default theme palette
	Background: lipgloss.AdaptiveColor{Light: "#f6f7f8", Dark: "#0b0f14"},
	Surface:    lipgloss.AdaptiveColor{Light: "#e4e7eb", Dark: "#151b23"},
	Primary:    lipgloss.AdaptiveColor{Light: "#006d5b", Dark: "#7de2c8"},
	Secondary:  lipgloss.AdaptiveColor{Light: "#334155", Dark: "#c7d2de"},
	Success:    lipgloss.AdaptiveColor{Light: "#1f7a3f", Dark: "#86efac"},
	Warning:    lipgloss.AdaptiveColor{Light: "#9a6700", Dark: "#f6c177"},
	Error:      lipgloss.AdaptiveColor{Light: "#b42318", Dark: "#ff8f8f"},
	Dim:        lipgloss.AdaptiveColor{Light: "#667085", Dark: "#7b8794"},
}

// defaultPalette is retained as an alias to the default relurpify-contrast palette.
var defaultPalette = relurpifyContrastPalette //nolint:gochecknoglobals // immutable default theme palette

var draculaPalette = Palette{ //nolint:gochecknoglobals // immutable dracula theme palette
	Background: lipgloss.AdaptiveColor{Light: "#f8f8f2", Dark: "#282a36"},
	Surface:    lipgloss.AdaptiveColor{Light: "#eaeaea", Dark: "#44475a"},
	Primary:    lipgloss.AdaptiveColor{Light: "#7c3aed", Dark: "#bd93f9"},
	Secondary:  lipgloss.AdaptiveColor{Light: "#6272a4", Dark: "#6272a4"},
	Success:    lipgloss.AdaptiveColor{Light: "#2e7d32", Dark: "#50fa7b"},
	Warning:    lipgloss.AdaptiveColor{Light: "#ef6c00", Dark: "#ffb86c"},
	Error:      lipgloss.AdaptiveColor{Light: "#c62828", Dark: "#ff5555"},
	Dim:        lipgloss.AdaptiveColor{Light: "#6272a4", Dark: "#6272a4"},
}

// New returns a theme with the provided palette.
func New(pal Palette) *Theme {
	return &Theme{pal: pal}
}

// Dracula returns the canonical Dracula-inspired theme.
func Dracula() *Theme {
	return New(draculaPalette).WithAccent(lipgloss.AdaptiveColor{Light: "#00838f", Dark: "#8be9fd"})
}

// WithPalette returns a clone of the theme with the palette replaced.
func (t *Theme) WithPalette(pal Palette) *Theme {
	clone := &Theme{pal: pal}
	if t != nil && t.accent != nil {
		accent := *t.accent
		clone.accent = &accent
	}
	return clone
}

// AppBackground renders a full-frame style with the theme background applied.
func (t *Theme) AppBackground() lipgloss.Style {
	if t == nil {
		t = Default()
	}
	return lipgloss.NewStyle().Background(t.pal.Background).Foreground(t.pal.Secondary)
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
	NotifInfo NotifKind = iota
	NotifHITL
	NotifSuccess
	NotifError
)

// Default returns the canonical default theme (relurpify-contrast).
func Default() *Theme {
	return RelurpifyContrast()
}

// RelurpifyContrast returns the default relurpify-contrast theme.
func RelurpifyContrast() *Theme {
	return New(relurpifyContrastPalette).WithAccent(lipgloss.AdaptiveColor{Light: "#00897b", Dark: "#56d4b5"})
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
	if t == nil {
		t = Default()
	}
	// Use a readable foreground against themed backgrounds.
	return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#282a36", Dark: "#f8f8f2"})
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

// ── Paradigm rendering ──────────────────────────────────────────────────────

// ParadigmGlyph returns the canonical 2-character glyph for the given paradigm.
func (t *Theme) ParadigmGlyph(p surface.Paradigm) string {
	return p.Describe().ShortGlyph
}

// ParadigmLabel returns the human-readable label for the given paradigm.
func (t *Theme) ParadigmLabel(p surface.Paradigm) string {
	return p.Describe().Label
}

// ParadigmDisplay returns the full display string (glyph + label) for the
// given paradigm, styled for use in compact contexts.
func (t *Theme) ParadigmDisplay(p surface.Paradigm) string {
	meta := p.Describe()
	return fmt.Sprintf("%s %s", meta.ShortGlyph, meta.Label)
}
