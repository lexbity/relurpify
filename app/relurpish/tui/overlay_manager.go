package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Overlay is a host-owned modal layer that captures keys and renders above
// Region 1.
type Overlay interface {
	Render(width, height int) string
	HandleKey(msg tea.KeyMsg) (tea.Cmd, bool)
}

// OverlayBounds describes the screen area occupied by an overlay.
type OverlayBounds struct {
	X int
	Y int
	W int
	H int
}

type overlayEntry struct {
	id      string
	overlay Overlay
}

// OverlayStack owns modal overlays in Z-order. Later pushes sit above earlier
// entries.
type OverlayStack struct {
	items []overlayEntry
	next  int
}

func NewOverlayStack() *OverlayStack {
	return &OverlayStack{}
}

func (s *OverlayStack) Clear() {
	if s == nil {
		return
	}
	s.items = nil
}

func (s *OverlayStack) Len() int {
	if s == nil {
		return 0
	}
	return len(s.items)
}

func (s *OverlayStack) Push(overlay Overlay) string {
	if s == nil || overlay == nil {
		return ""
	}
	s.next++
	s.items = append(s.items, overlayEntry{id: overlayID(s.next), overlay: overlay})
	return s.items[len(s.items)-1].id
}

func (s *OverlayStack) PushExclusive(overlay Overlay) string {
	if s == nil {
		return ""
	}
	s.Clear()
	return s.Push(overlay)
}

func (s *OverlayStack) Pop() (Overlay, bool) {
	if s == nil || len(s.items) == 0 {
		return nil, false
	}
	last := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return last.overlay, true
}

func (s *OverlayStack) Remove(id string) bool {
	if s == nil || id == "" {
		return false
	}
	for i := len(s.items) - 1; i >= 0; i-- {
		if s.items[i].id == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return true
		}
	}
	return false
}

func (s *OverlayStack) Top() Overlay {
	if s == nil || len(s.items) == 0 {
		return nil
	}
	return s.items[len(s.items)-1].overlay
}

func (s *OverlayStack) HandleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if s == nil || len(s.items) == 0 {
		return nil, false
	}
	top := s.items[len(s.items)-1].overlay
	cmd, handled := top.HandleKey(msg)
	return cmd, handled
}

func (s *OverlayStack) Render(width, height int) string {
	if s == nil || len(s.items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(s.items))
	for _, item := range s.items {
		if item.overlay == nil {
			continue
		}
		rendered := normalizeOverlayText(item.overlay.Render(width, height))
		if rendered == "" {
			continue
		}
		parts = append(parts, rendered)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

func normalizeOverlayText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	return strings.Join(lines, "\n")
}

func overlayID(n int) string {
	return fmt.Sprintf("overlay-%d", n)
}

type overlayFunc struct {
	render func(width, height int) string
	handle func(msg tea.KeyMsg) (tea.Cmd, bool)
}

func (o overlayFunc) Render(width, height int) string {
	if o.render == nil {
		return ""
	}
	return o.render(width, height)
}

func (o overlayFunc) HandleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if o.handle == nil {
		return nil, false
	}
	return o.handle(msg)
}
