package tui

import "codeburg.org/lexbit/relurpify/app/relurpish/theme"

// ThemeSetter is implemented by any component that accepts a theme.
type ThemeSetter interface {
	SetTheme(th *theme.Theme)
}
