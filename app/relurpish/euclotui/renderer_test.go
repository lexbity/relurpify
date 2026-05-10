package euclotui

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

func TestRenderInteractionFrameStructuredSelection(t *testing.T) {
	frame := interaction.InteractionFrame{
		Type:          interaction.FrameCapabilitySelection,
		Kind:          interaction.FrameCapabilitySelection,
		Question:      "Choose the capability.",
		Choices:       []string{"review", "implement"},
		DefaultChoice: "review",
		Slots: []interaction.ActionSlot{
			{ID: "review", Label: "Review", Default: true, Risk: "low"},
			{ID: "implement", Label: "Implement", Risk: "medium"},
		},
		Payload: map[string]any{
			"capability_id": "euclo:cap.intent.clarify",
		},
	}

	msg := RenderInteractionFrame(frame)
	if got := msg.Content.Text; !strings.Contains(got, "Capability Selection") {
		t.Fatalf("expected structured title in %q", got)
	}
	if got := msg.Content.Text; !strings.Contains(got, "Choose the capability.") {
		t.Fatalf("expected question in %q", got)
	}
	if got := msg.Content.Text; !strings.Contains(got, "Review") || !strings.Contains(got, "Implement") {
		t.Fatalf("expected action labels in %q", got)
	}
	if got := msg.Content.Text; !strings.Contains(got, "capability_id: euclo:cap.intent.clarify") {
		t.Fatalf("expected payload field in %q", got)
	}
}

func TestNotificationAllowsFreetextForClarificationAnswerFrame(t *testing.T) {
	item := notificationItemFromFrame("notif-1", NotifKindInteraction, interaction.InteractionFrame{
		Type:     interaction.FrameIntentClarification,
		Kind:     interaction.FrameIntentClarification,
		Question: "What should I change?",
		Slots: []interaction.ActionSlot{
			{ID: "answer", Label: "Answer", Default: true},
		},
	}, nil)

	if !notificationAllowsFreetext(item) {
		t.Fatal("expected freetext interaction to be allowed")
	}
}
