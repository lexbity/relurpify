package tui

import (
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

func TestNotificationQueue_PushInteraction(t *testing.T) {
	q := &NotificationQueue{}
	frame := interaction.InteractionFrame{
		Kind:  interaction.FrameProposal,
		Mode:  "code",
		Phase: "intent",
		Actions: []interaction.ActionSlot{
			{ID: "confirm", Label: "Confirm", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
			{ID: "skip", Label: "Skip", Kind: interaction.ActionConfirm},
		},
	}

	id := q.PushInteraction(frame)
	if id == "" {
		t.Error("expected non-empty ID")
	}
	if q.Len() != 1 {
		t.Errorf("len: got %d, want 1", q.Len())
	}

	item, ok := q.Current()
	if !ok {
		t.Fatal("expected current item")
	}
	if item.Kind != NotifKindInteraction {
		t.Errorf("kind: got %q", item.Kind)
	}
	if item.Extra["action_count"] != "2" {
		t.Errorf("action_count: got %q", item.Extra["action_count"])
	}
	if item.Extra["default_action"] != "confirm" {
		t.Errorf("default_action: got %q", item.Extra["default_action"])
	}
	if item.Extra["action_0_shortcut"] != "y" {
		t.Errorf("shortcut: got %q", item.Extra["action_0_shortcut"])
	}
}

func TestResolveInteractionKey_Shortcut(t *testing.T) {
	item := NotificationItem{
		Kind: NotifKindInteraction,
		Extra: map[string]string{
			"action_count":      "2",
			"action_0_id":       "confirm",
			"action_0_label":    "Confirm",
			"action_0_shortcut": "y",
			"action_0_default":  "true",
			"action_1_id":       "skip",
			"action_1_label":    "Skip",
			"action_1_shortcut": "",
			"default_action":    "confirm",
		},
	}

	// Shortcut match.
	resp, ok := ResolveInteractionKey(item, "y")
	if !ok {
		t.Fatal("expected match for shortcut 'y'")
	}
	if resp.ActionID != "confirm" {
		t.Errorf("action: got %q", resp.ActionID)
	}
}

func TestResolveInteractionKey_NumberKey(t *testing.T) {
	item := NotificationItem{
		Kind: NotifKindInteraction,
		Extra: map[string]string{
			"action_count":      "2",
			"action_0_id":       "confirm",
			"action_0_label":    "Confirm",
			"action_0_shortcut": "",
			"action_1_id":       "skip",
			"action_1_label":    "Skip",
			"action_1_shortcut": "",
		},
	}

	resp, ok := ResolveInteractionKey(item, "2")
	if !ok {
		t.Fatal("expected match for number key '2'")
	}
	if resp.ActionID != "skip" {
		t.Errorf("action: got %q", resp.ActionID)
	}
}

func TestResolveInteractionKey_Enter(t *testing.T) {
	item := NotificationItem{
		Kind: NotifKindInteraction,
		Extra: map[string]string{
			"action_count":   "2",
			"action_0_id":    "confirm",
			"action_1_id":    "skip",
			"default_action": "confirm",
		},
	}

	resp, ok := ResolveInteractionKey(item, "enter")
	if !ok {
		t.Fatal("expected match for enter")
	}
	if resp.ActionID != "confirm" {
		t.Errorf("action: got %q", resp.ActionID)
	}
}

func TestResolveInteractionKey_NoMatch(t *testing.T) {
	item := NotificationItem{
		Kind: NotifKindInteraction,
		Extra: map[string]string{
			"action_count": "1",
			"action_0_id":  "confirm",
		},
	}

	_, ok := ResolveInteractionKey(item, "x")
	if ok {
		t.Error("expected no match for unbound key")
	}
}

func TestRenderInteractionNotification(t *testing.T) {
	item := NotificationItem{
		Kind: NotifKindInteraction,
		Msg:  "[code/intent] Review proposal",
		Extra: map[string]string{
			"action_count":      "2",
			"action_0_id":       "confirm",
			"action_0_label":    "Confirm",
			"action_0_shortcut": "y",
			"action_0_kind":     "confirm",
			"action_0_default":  "true",
			"action_1_id":       "skip",
			"action_1_label":    "Skip",
			"action_1_shortcut": "",
			"action_1_kind":     "confirm",
			"default_action":    "confirm",
		},
	}

	rendered := RenderInteractionNotification(item)
	if rendered == "" {
		t.Error("expected non-empty render")
	}
}
