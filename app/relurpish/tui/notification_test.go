package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNotificationBarResolvesInteractionSlotByNumber(t *testing.T) {
	q := &NotificationQueue{}
	q.Push(NotificationItem{
		ID:   "notif-1",
		Kind: NotifKindInteraction,
		Msg:  "intent clarification",
		Extra: map[string]string{
			"frame_id":     "frame-1",
			"frame_type":   "intent_clarification",
			"slot_count":   "2",
			"slot_0_id":    "review",
			"slot_0_label": "Review",
			"slot_1_id":    "implement",
			"slot_1_label": "Implement",
			"default_slot": "review",
		},
	})

	bar := NewNotificationBar(q)
	bar, cmd := bar.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if bar == nil {
		t.Fatal("expected notification bar")
	}
	if cmd == nil {
		t.Fatal("expected resolve command")
	}
	msg := cmd()
	resolved, ok := msg.(NotifInteractionResolveMsg)
	if !ok {
		t.Fatalf("expected NotifInteractionResolveMsg, got %#v", msg)
	}
	if resolved.NotificationID != "notif-1" {
		t.Fatalf("notification id = %q", resolved.NotificationID)
	}
	if resolved.FrameID != "frame-1" {
		t.Fatalf("frame id = %q", resolved.FrameID)
	}
	if resolved.ChoiceID != "implement" {
		t.Fatalf("choice id = %q", resolved.ChoiceID)
	}
	if q.Len() != 0 {
		t.Fatalf("expected notification queue to be drained, got %d", q.Len())
	}
}
