package demo_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/app/relurpish/demo"
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	tea "github.com/charmbracelet/bubbletea"
)

type manualClock struct{ timers chan chan time.Time }

func (c *manualClock) After(time.Duration) <-chan time.Time {
	timer := make(chan time.Time, 1)
	c.timers <- timer
	return timer
}

func model(t *testing.T, scenario string) (tui.RootModel, *demo.Session) {
	t.Helper()
	session, err := demo.New(scenario, &manualClock{timers: make(chan chan time.Time, 16)})
	if err != nil {
		t.Fatal(err)
	}
	m := tui.NewRootModel(t.Context(), tui.HostOptions{DemoScenario: scenario, Conversation: session, Doctor: session, Initializer: session, Recipes: session, Configuration: session, Services: session, Providers: session})
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = update(m, tui.DoctorStatusMsg{Report: session.BuildDoctorReport(t.Context())})
	return m, session
}

func update(m tui.RootModel, msg tea.Msg) (tui.RootModel, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(tui.RootModel), cmd
}
func key(m tui.RootModel, kind tea.KeyType) (tui.RootModel, tea.Cmd) {
	return update(m, tea.KeyMsg{Type: kind})
}

func TestV2DemoDecisionUsesSharedHostAndPreservesDraft(t *testing.T) {
	m, session := model(t, "question")
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a request stays a draft")})
	if session.Current().Decision == nil {
		t.Fatal("typing resolved a demo question")
	}
	m, _ = key(m, tea.KeyCtrlG)
	if !strings.Contains(m.View(), "Which behavior") || !strings.Contains(m.View(), "DEMO") {
		t.Fatal("shared attention view was not deliberately opened/labeled")
	}
	m, _ = key(m, tea.KeyDown)
	m, cmd := key(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("focused answer did not invoke the application port")
	}
	m, _ = update(m, cmd())
	if session.Current().Decision != nil || m.InteractionState().Draft != "a request stays a draft" {
		t.Fatal("answer was not recorded or stole the request draft")
	}
}

func TestV2DemoInitializationReviewCancelAndApply(t *testing.T) {
	m, session := model(t, "first-launch")
	m, _ = key(m, tea.KeyF6)
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if !strings.Contains(m.View(), "Review initialization") {
		t.Fatal("initialization bypassed review")
	}
	m, _ = key(m, tea.KeyEsc)
	if session.BuildDoctorReport(t.Context()).ConfigExists {
		t.Fatal("cancel initialized the workspace")
	}
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m, cmd := key(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("review confirmation did not initialize demo state")
	}
	m, _ = update(m, cmd())
	if !session.BuildDoctorReport(t.Context()).ConfigExists || !strings.Contains(m.View(), "Ready") {
		t.Fatal("confirmed initialization was not reflected on reopening Doctor")
	}
}

func TestV2DemoClockCancellationAndSnapshotIsolation(t *testing.T) {
	clock := &manualClock{timers: make(chan chan time.Time, 16)}
	session, err := demo.New("quiet", clock)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan tui.FrontendSnapshot, 1)
	go func() { snapshot, _ := session.Next(ctx); result <- snapshot }()
	select {
	case <-clock.timers:
		t.Fatal("idle provider scheduled work")
	case <-time.After(20 * time.Millisecond):
	}
	if _, err := session.Submit(ctx, "repair"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("submission did not wake observation")
	}
	go func() { snapshot, _ := session.Next(ctx); result <- snapshot }()
	var tick chan time.Time
	select {
	case tick = <-clock.timers:
	case <-time.After(time.Second):
		t.Fatal("running provider did not schedule an application event")
	}
	if _, err := session.Cancel(ctx); err != nil {
		t.Fatal(err)
	}
	tick <- time.Now()
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("cancel did not release pending observation")
	}
	got := session.Current()
	if got.Running || !strings.Contains(got.Status, "canceled") {
		t.Fatal("late timer resurrected canceled execution")
	}
	got.Circuit.Steps[1].Dependencies[0] = "corrupt"
	if session.Current().Circuit.Steps[1].Dependencies[0] != "understand" {
		t.Fatal("snapshot leaked mutable data")
	}
}

func TestV2DemoRequestAcknowledgementPreservesNewerDraft(t *testing.T) {
	m, session := model(t, "quiet")
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("repair")})
	m, command := key(m, tea.KeyEnter)
	if command == nil {
		t.Fatal("composer did not emit submission")
	}
	m, submit := update(m, command())
	if submit == nil {
		t.Fatal("host did not call demo application port")
	}
	// Edit before the asynchronous acknowledgement reaches the host.
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" later")})
	m, _ = update(m, submit())
	if m.InteractionState().Draft != "repair later" || !session.Current().Running {
		t.Fatal("late acknowledgement erased a newer draft or failed admission")
	}
}

func TestV2DemoRejectsLiveFactory(t *testing.T) {
	session, err := demo.New("quiet", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("demo accepted a factory that could construct live panels")
		}
	}()
	tui.NewRootModel(t.Context(), tui.HostOptions{DemoScenario: "quiet", Conversation: session, SurfaceFactory: tui.NewDefaultSurfaceFactory()})
}
