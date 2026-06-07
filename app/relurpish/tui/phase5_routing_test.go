package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

type recordingOverlay struct {
	keys []string
}

func (o *recordingOverlay) Render(width, height int) string {
	_, _ = width, height
	return "overlay"
}

func (o *recordingOverlay) HandleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	o.keys = append(o.keys, msg.String())
	return nil, true
}

type gatedSubmitChatPane struct {
	fakeChatPane
	submits []string
}

func (p *gatedSubmitChatPane) HandleInputSubmit(value string) tea.Cmd {
	p.submits = append(p.submits, value)
	return nil
}

func TestOverlayPrecedenceBlocksRegion1Routing(t *testing.T) {
	chat := &recordingChatPane{}
	surface := &fakeSurface{name: "guest", chat: chat}
	factory := &countingFactory{shared: surface}
	m := newRootModel(nil, factory)
	m.setFocus(FocusRegionRegion1)
	m.openAgentPicker()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	rm := updated.(RootModel)

	if len(chat.keys) != 0 {
		t.Fatalf("expected region 1 to be bypassed, got %#v", chat.keys)
	}
	if rm.agentPicker != nil && rm.agentPicker.IsOpen() {
		t.Fatal("expected agent picker to close after overlay key")
	}
	if !rm.focus.State().InRegion1() {
		t.Fatal("expected focus to remain in region 1 while overlay handled the key")
	}
}

func TestHITLRowAnswerAndDismissAreHandledByHost(t *testing.T) {
	q := &NotificationQueue{}
	frame := interaction.NewClarificationFrame("task-1", "session-1", "Pick one", []string{"review", "implement"}, nil)
	m := RootModel{
		notifQ:            q,
		hitlRow:           &HITLRow{},
		interactionFrames: make(map[string]*interaction.InteractionFrame),
	}
	m.trackInteractionFrame("notif-1", *frame)
	q.Push(NotificationItem{ID: "notif-1", Kind: NotifKindInteraction, Msg: "clarification"})
	m.openInteractionGuidance("notif-1", *frame)
	tracked := m.interactionFrames["notif-1"]

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	rm := updated.(RootModel)
	if cmd == nil {
		t.Fatal("expected HITL row answer command")
	}
	updated, _ = rm.Update(cmd())
	rm = updated.(RootModel)

	if rm.hitlRow.Active() {
		t.Fatal("expected HITL row to close after answer")
	}
	if tracked == nil || tracked.Response == nil || tracked.Response.ChosenSlot != "review" {
		t.Fatalf("expected review response, got %#v", tracked)
	}
	if q.Len() != 0 {
		t.Fatalf("expected notification queue to drain, got %d", q.Len())
	}

	m2 := RootModel{
		notifQ:            &NotificationQueue{},
		hitlRow:           &HITLRow{},
		interactionFrames: make(map[string]*interaction.InteractionFrame),
	}
	frame2 := interaction.NewClarificationFrame("task-2", "session-2", "Pick one", []string{"review", "implement"}, nil)
	m2.trackInteractionFrame("notif-2", *frame2)
	m2.openInteractionGuidance("notif-2", *frame2)
	tracked2 := m2.interactionFrames["notif-2"]

	updated, cmd = m2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	rm = updated.(RootModel)
	if cmd == nil {
		t.Fatal("expected HITL row dismiss command")
	}
	updated, _ = rm.Update(cmd())
	rm = updated.(RootModel)

	if rm.hitlRow.Active() {
		t.Fatal("expected HITL row to dismiss on esc")
	}
	if tracked2 != nil && tracked2.Response != nil {
		t.Fatalf("expected dismissed frame to remain unresolved, got %#v", tracked2.Response)
	}
}

func TestInputSubmissionBlockedWhileRunIsActive(t *testing.T) {
	chat := &gatedSubmitChatPane{}
	m := RootModel{
		chat:      chat,
		inputGate: &InputGate{},
	}
	m.inputGate.SetActive(true)

	updated, cmd := m.handleInputSubmitted(InputSubmittedMsg{Value: "run this", Prefix: ">"})
	rm := updated.(RootModel)

	if cmd != nil {
		t.Fatalf("expected nil command while gated, got %v", cmd)
	}
	if len(chat.submits) != 0 {
		t.Fatalf("expected chat submit to be blocked, got %#v", chat.submits)
	}
	if !rm.inputGate.Active() {
		t.Fatal("expected input gate to remain active")
	}
}
