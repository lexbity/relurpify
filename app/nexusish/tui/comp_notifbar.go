package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type NotificationKind int

const (
	NotificationInfo NotificationKind = iota
	NotificationWarning
	NotificationError
	NotificationAction
)

type Notification struct {
	Kind   NotificationKind
	Title  string
	Value  string
	TTL    time.Duration
	Sticky bool

	ExpiresAt time.Time
}

type NotificationBar struct {
	width  int
	items  []Notification
	active int
	now    time.Time
}

func NewNotificationBar() *NotificationBar {
	return &NotificationBar{}
}

func (b *NotificationBar) SetWidth(width int) {
	b.width = width
}

func (b *NotificationBar) Active() bool {
	return b != nil && len(b.items) > 0
}

func (b *NotificationBar) Push(n Notification) {
	if b == nil {
		return
	}
	if n.TTL == 0 && !n.Sticky {
		n.TTL = 8 * time.Second
	}
	if n.TTL > 0 {
		n.TTL += time.Second
	}
	b.items = append(b.items, n)
}

func (b *NotificationBar) DismissActive() {
	if b == nil || len(b.items) == 0 {
		return
	}
	b.items = append(b.items[:b.active], b.items[b.active+1:]...)
	if b.active >= len(b.items) && b.active > 0 {
		b.active--
	}
}

func (b *NotificationBar) Tick(now time.Time) {
	if b == nil {
		return
	}
	b.now = now
	next := b.items[:0]
	for _, item := range b.items {
		if item.Sticky || item.TTL <= 0 {
			next = append(next, item)
			continue
		}
		item.TTL -= refreshInterval
		if item.TTL > 0 {
			next = append(next, item)
		}
	}
	b.items = next
	if b.active >= len(b.items) && b.active > 0 {
		b.active--
	}
}

func (b *NotificationBar) SyncState(state RuntimeState) {
	if b == nil {
		return
	}
	if len(state.PendingPairings) > 0 {
		first := state.PendingPairings[0]
		title := fmt.Sprintf("Pending pairing %s", first.Code)
		if len(b.items) == 0 || b.items[0].Value != first.Code {
			b.items = append([]Notification{{
				Kind:      NotificationAction,
				Title:     title,
				Value:     first.Code,
				Sticky:    true,
				ExpiresAt: first.ExpiresAt,
			}}, trimPairingNotifications(b.items)...)
		} else {
			b.items[0].Title = title
			b.items[0].ExpiresAt = first.ExpiresAt
		}
		return
	}
	b.items = trimPairingNotifications(b.items)
	if b.active >= len(b.items) && b.active > 0 {
		b.active--
	}
}

func trimPairingNotifications(items []Notification) []Notification {
	out := items[:0]
	for _, item := range items {
		if item.Kind == NotificationAction {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (b *NotificationBar) HandleKey(msg tea.KeyMsg) (notifActionMsg, bool) {
	if b == nil || len(b.items) == 0 {
		return notifActionMsg{}, false
	}
	item := b.items[b.active]
	switch msg.String() {
	case "tab":
		if len(b.items) > 1 {
			b.active = (b.active + 1) % len(b.items)
			return notifActionMsg{}, true
		}
	case "d":
		return notifActionMsg{action: "dismiss"}, true
	case "a":
		if item.Kind == NotificationAction {
			return notifActionMsg{action: "approve", value: item.Value}, true
		}
	case "x":
		if item.Kind == NotificationAction {
			return notifActionMsg{action: "reject", value: item.Value}, true
		}
	}
	return notifActionMsg{}, false
}

func (b NotificationBar) View() string {
	if len(b.items) == 0 {
		return ""
	}
	item := b.items[b.active]
	content := item.displayTitle(b.now)
	if item.Kind == NotificationAction {
		content += "  " + mutedStyle.Render("[a] approve  [x] reject  [d] dismiss")
	}
	style := notifInfoStyle
	switch item.Kind {
	case NotificationWarning:
		style = notifWarnStyle
	case NotificationError:
		style = notifErrorStyle
	case NotificationAction:
		style = notifActionStyle
	}
	return style.Width(b.width).Render(content)
}

func (n Notification) displayTitle(now time.Time) string {
	if n.ExpiresAt.IsZero() {
		return n.Title
	}
	if now.IsZero() {
		now = time.Now()
	}
	if !n.ExpiresAt.After(now) {
		return fmt.Sprintf("%s expired", n.Title)
	}
	return fmt.Sprintf("%s expires in %s", n.Title, n.ExpiresAt.Sub(now).Round(time.Second))
}
