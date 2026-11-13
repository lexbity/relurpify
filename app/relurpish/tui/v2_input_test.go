package tui

import (
	"context"
	"strings"
	"testing"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestV2ApprovalCannotStealTyping(t *testing.T) {
	m, _ := newFocusTestModel()
	m.notifQ.Push(NotificationItem{ID: "pending", Kind: NotifKindHITL, Msg: "approve write"})
	m.inputBar.SetValue("draft ")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(RootModel)
	if cmd != nil {
		if _, approved := cmd().(NotifHITLApproveMsg); approved {
			t.Fatal("ordinary typing granted persistent permission")
		}
	}
	if m.inputBar.Value() != "draft a" || m.notifQ.Len() != 1 {
		t.Fatalf("typing changed decision or draft: %q, %d", m.inputBar.Value(), m.notifQ.Len())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	m = updated.(RootModel)
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("deliberately focused approval has no action")
	}
	if _, ok := cmd().(NotifHITLApproveMsg); !ok {
		t.Fatal("focused confirmation did not produce an approval")
	}
}

func TestV2BlockedAndStaleSubmissionRetainsDraft(t *testing.T) {
	m, _ := newFocusTestModel()
	m.startupLocked = true
	m.inputBar.SetValue("my request")
	updated, cmd := m.handleInputSubmitted(InputSubmittedMsg{Value: "my request"})
	m = updated.(RootModel)
	if cmd != nil || m.inputBar.Value() != "my request" {
		t.Fatal("blocked submit lost draft or dispatched")
	}
	m.inputBar.SetValue("newer draft")
	m.inputBar.Accept("my request")
	if m.inputBar.Value() != "newer draft" {
		t.Fatal("late acknowledgement erased a newer edit")
	}
}

func TestV2PlainTextEverySurface(t *testing.T) {
	for _, tab := range []TabID{TabChat, TabDoctor, TabSandbox, TabKeybindings, TabAIProvider} {
		b := NewInputBar()
		b.SetValue("  :this remains literal?  ")
		_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyEnter}, tab)
		if cmd == nil {
			t.Fatalf("%s did not submit", tab)
		}
		msg, ok := cmd().(InputSubmittedMsg)
		if !ok || msg.Value != "  :this remains literal?  " || msg.Prefix != "" {
			t.Fatalf("%s reinterpreted request: %#v", tab, msg)
		}
		if b.Value() != msg.Value {
			t.Fatal("composer cleared before host admission")
		}
	}
}

func TestV2PasteDoesNotSubmit(t *testing.T) {
	b := NewInputBar()
	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("first\nsecond"), Paste: true}, TabChat)
	if cmd != nil {
		if _, submitted := cmd().(InputSubmittedMsg); submitted {
			t.Fatal("paste submitted a request")
		}
	}
	if b.Value() != "first\nsecond" {
		t.Fatalf("multiline paste lost text: %q", b.Value())
	}
	before := b.Value()
	_, cmd = b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strings.Repeat("x", 65537)), Paste: true}, TabChat)
	if cmd == nil || b.Value() != before {
		t.Fatal("oversize paste must fail atomically")
	}
}

func TestV2ViewIsPure(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {39, 9}, {60, 16}, {80, 24}, {120, 40}, {240, 80}} {
		m, _ := newFocusTestModel()
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = updated.(RootModel)
		m.inputBar.SetValue("界 e\u0301 👩‍💻")
		before := m.inputBar.Value()
		a, b := m.View(), m.View()
		if a != b || m.inputBar.Value() != before || m.inputGate.Active() {
			t.Fatal("render mutated state")
		}
		lines := strings.Split(a, "\n")
		if len(lines) > size[1] {
			t.Fatalf("height %d exceeds %d", len(lines), size[1])
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("width %d exceeds %d: %q", ansi.StringWidth(line), size[0], line)
			}
		}
	}
}

func TestV2DraftSurvivesNavigation(t *testing.T) {
	m, _ := newFocusTestModel()
	m.inputBar.SetValue("unsent request")
	for _, tab := range []TabID{TabDoctor, TabSandbox, TabAIProvider, TabChat} {
		m.setActiveTab(tab)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		m = updated.(RootModel)
		if m.inputBar.Value() != "unsent request" {
			t.Fatalf("navigation to %s erased draft", tab)
		}
	}
}

type readOnlyDoctor struct{ calls int }

func (d *readOnlyDoctor) BuildDoctorReport(ctx context.Context) runtimesvc.DoctorReport {
	d.calls++
	if _, ok := ctx.Deadline(); !ok {
		panic("bootstrap probe is unbounded")
	}
	return runtimesvc.DoctorReport{}
}

func TestV2DoctorWithoutRuntime(t *testing.T) {
	m, _ := newFocusTestModel()
	doctor := &readOnlyDoctor{}
	m.runtime = nil
	m.doctorReader = doctor
	m.startupLocked = true
	m.inputBar.SetValue("keep this")
	cmd := m.startupDoctorReportCmd()
	if cmd == nil {
		t.Fatal("Doctor requires execution runtime")
	}
	updated, _ := m.Update(cmd())
	m = updated.(RootModel)
	if doctor.calls != 1 || !m.startupLocked || m.inputBar.Value() != "keep this" {
		t.Fatal("read-only startup changed execution or draft state")
	}
}

func TestV2TerminalContentCannotControlScreen(t *testing.T) {
	got := terminalText("hello\x1b]52;c;payload\a\x1b[2Jworld\u202e")
	if got != "helloworld" {
		t.Fatalf("control content escaped sanitization: %q", got)
	}
}

func TestV2TextBatchesAreNeverControlKeys(t *testing.T) {
	for _, text := range []string{"delete", "home", "f1", "1 is ordinary text", "ctrl+c"} {
		m, _ := newFocusTestModel()
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
		m = updated.(RootModel)
		if m.inputBar.Value() != text || m.showHelp {
			t.Fatalf("text batch %q interpreted as keys: draft=%q help=%v", text, m.inputBar.Value(), m.showHelp)
		}
	}
}
