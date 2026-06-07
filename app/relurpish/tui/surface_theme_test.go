package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
)

// nilSurface implements AgentSurface with Theme() returning nil.
type nilSurface struct{ genericSurface }

func (nilSurface) Theme() *theme.Theme { return nil }

// customSurface implements AgentSurface with a non-nil theme.
type customSurface struct {
	genericSurface
	custom *theme.Theme
}

func (s *customSurface) Theme() *theme.Theme { return s.custom }

func TestSurfaceThemeNilReturnsDefault(t *testing.T) {
	d := resolveSurfaceTheme(nilSurface{})
	if d == nil {
		t.Fatal("resolveSurfaceTheme(nil surface) returned nil")
	}
}

func TestSurfaceThemeCustomReturnsCustom(t *testing.T) {
	custom := theme.Default().WithAccent(lipgloss.AdaptiveColor{Light: "#ff0000", Dark: "#ff0000"})
	s := &customSurface{custom: custom}
	got := resolveSurfaceTheme(s)
	if got == nil {
		t.Fatal("resolveSurfaceTheme(custom) returned nil")
	}
}

func TestSurfaceThemeNilInterfaceReturnsDefault(t *testing.T) {
	d := resolveSurfaceTheme(nil)
	if d == nil {
		t.Fatal("resolveSurfaceTheme(nil interface) returned nil")
	}
}

func TestGenericSurfaceTheme(t *testing.T) {
	gs := newGenericSurface()
	th := gs.Theme()
	if th != nil {
		t.Error("genericSurface.Theme() should return nil (host default)")
	}
}

func TestPropagateSurfaceThemeNilSurface(t *testing.T) {
	m := &RootModel{}
	propagateSurfaceTheme(m, nilSurface{})
	if m.th == nil {
		t.Fatal("propagateSurfaceTheme with nil surface should set default theme")
	}
}

func TestPropagateSurfaceThemeUpdatesModelTheme(t *testing.T) {
	custom := theme.Default().WithAccent(lipgloss.AdaptiveColor{Light: "#ff0000", Dark: "#ff0000"})
	s := &customSurface{custom: custom}
	m := &RootModel{}
	propagateSurfaceTheme(m, s)
	if m.th == nil {
		t.Error("model theme should be set after propagateSurfaceTheme")
	}
}
