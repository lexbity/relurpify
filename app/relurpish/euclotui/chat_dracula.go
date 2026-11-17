package euclotui

import (
	"github.com/charmbracelet/lipgloss"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
)

// DraculaTheme builds the Dracula chat theme (available as an explicit theme).
func DraculaTheme() *theme.Theme {
	return theme.Dracula()
}

// chatPalette provides colors for chat rendering derived from the active theme.
type chatPalette struct {
	bg      lipgloss.AdaptiveColor
	surface lipgloss.AdaptiveColor
	fg      lipgloss.AdaptiveColor
	muted   lipgloss.AdaptiveColor
	purple  lipgloss.AdaptiveColor
	pink    lipgloss.AdaptiveColor
	cyan    lipgloss.AdaptiveColor
	green   lipgloss.AdaptiveColor
	orange  lipgloss.AdaptiveColor
	red     lipgloss.AdaptiveColor
	yellow  lipgloss.AdaptiveColor
}

func chatPaletteFromTheme(th *theme.Theme) chatPalette {
	if th == nil {
		th = theme.Default()
	}
	pal := th.Palette()
	fg := lipgloss.AdaptiveColor{Light: "#282a36", Dark: "#f8f8f2"}
	if pal.Secondary.Dark != "" || pal.Secondary.Light != "" {
		// keep readable fg independent of secondary in dark themes
		fg = lipgloss.AdaptiveColor{Light: "#111827", Dark: "#f8f8f2"}
	}
	return chatPalette{
		bg:      pal.Background,
		surface: pal.Surface,
		fg:      fg,
		muted:   pal.Dim,
		purple:  pal.Primary,
		pink:    pal.Error,
		cyan:    pal.Primary,
		green:   pal.Success,
		orange:  pal.Warning,
		red:     pal.Error,
		yellow:  pal.Warning,
	}
}

// newDraculaStyles kept as compatibility helper for older code paths; prefers theme-derived palette.
func newDraculaStyles() chatPalette {
	return chatPaletteFromTheme(theme.Default())
}
