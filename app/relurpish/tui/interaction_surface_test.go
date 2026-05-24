package tui

import (
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

func TestEucloSurfaceOpensGuidanceForFreeTextClarification(t *testing.T) {
	q := &NotificationQueue{}
	m := RootModel{
		notifQ:            q,
		hitlRow:           &HITLRow{},
		interactionFrames: make(map[string]*interaction.InteractionFrame),
	}

	frame := interaction.NewAskUserFrame("task-1", "session-1", "What should I change?", nil)
	q.Push(NotificationItem{ID: "notif-1", Kind: NotifKindInteraction, Msg: "intent clarification"})
	m.trackInteractionFrame("notif-1", *frame)
	m.OpenInteractionGuidance("notif-1", *frame)

	if q.Len() != 1 {
		t.Fatalf("expected one notification, got %d", q.Len())
	}
	if m.hitlRow == nil || !m.hitlRow.Active() {
		t.Fatal("expected hitl row to open for freetext clarification")
	}
	if m.hitlRow.FrameID() != "notif-1" {
		t.Fatalf("frame id = %q, want %q", m.hitlRow.FrameID(), "notif-1")
	}
	if _, ok := m.interactionFrames["notif-1"]; !ok {
		t.Fatalf("expected interaction frame tracked under notification id %q", "notif-1")
	}
}

func TestResolvePendingInteractionRecordsFrameResponse(t *testing.T) {
	q := &NotificationQueue{}
	frame := interaction.NewClarificationFrame("task-1", "session-1", "Pick one", []string{"review", "implement"}, nil)
	m := RootModel{
		notifQ:            q,
		interactionFrames: make(map[string]*interaction.InteractionFrame),
	}
	m.trackInteractionFrame("notif-1", *frame)
	tracked := m.interactionFrames["notif-1"]
	q.Push(NotificationItem{ID: "notif-1", Kind: NotifKindInteraction, Msg: "clarification"})

	if !m.resolvePendingInteraction("notif-1", "implement", "") {
		t.Fatal("expected interaction to resolve")
	}
	if q.Len() != 0 {
		t.Fatalf("expected queue to be drained, got %d", q.Len())
	}
	if tracked == nil || tracked.Response == nil {
		t.Fatal("expected frame response to be recorded")
	}
	if got := tracked.Response.ChosenSlot; got != "implement" {
		t.Fatalf("chosen slot = %q", got)
	}
}

func TestDeferPendingInteractionLeavesFrameUnresolved(t *testing.T) {
	q := &NotificationQueue{}
	frame := interaction.NewClarificationFrame("task-1", "session-1", "Pick one", []string{"review", "implement"}, nil)
	m := RootModel{
		notifQ:            q,
		interactionFrames: make(map[string]*interaction.InteractionFrame),
	}
	m.trackInteractionFrame("notif-1", *frame)
	tracked := m.interactionFrames["notif-1"]
	q.Push(NotificationItem{ID: "notif-1", Kind: NotifKindInteraction, Msg: "clarification"})

	if !m.deferPendingInteraction("notif-1") {
		t.Fatal("expected interaction to defer")
	}
	if q.Len() != 0 {
		t.Fatalf("expected queue to be drained, got %d", q.Len())
	}
	if tracked != nil && tracked.Response != nil {
		t.Fatalf("expected deferred frame to remain unresolved, got %#v", tracked.Response)
	}
}
