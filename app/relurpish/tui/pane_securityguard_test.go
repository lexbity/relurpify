package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"codeburg.org/lexbit/relurpify/userconfig/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const guardShellFixture = `schema: relurpify/policy/shell/v1
rules:
  - id: delete
    pattern: 'rm\s+-rf\s+/'
    action: block
    reason: destructive
`
const guardIngestionFixture = `schema: relurpify/policy/ingestion/v1
rules:
  - id: external
    enabled: true
    effect:
      action: deny
`

type guardReaderFixture struct {
	reads int
	err   error
}

func (r *guardReaderFixture) GuardPolicy(ctx context.Context, section config.GuardSection) (GuardPolicySnapshot, error) {
	r.reads++
	if r.err != nil {
		return GuardPolicySnapshot{}, r.err
	}
	if err := ctx.Err(); err != nil {
		return GuardPolicySnapshot{}, err
	}
	source := guardShellFixture
	if section == config.GuardIngestion {
		source = guardIngestionFixture
	}
	return GuardPolicySnapshot{Workspace: "fixture", Document: config.GuardPolicyDocument{Section: section, Source: source, Revision: "fixture-revision"}}, nil
}
func loadGuardFixture(t *testing.T) (*SecurityGuardPane, *guardReaderFixture) {
	t.Helper()
	reader := &guardReaderFixture{}
	p := NewSecurityGuardPane(t.Context(), reader, false)
	p.SetSize(96, 22)
	cmd := p.open()
	if cmd == nil || reader.reads != 0 {
		t.Fatal("loading was synchronous or not dispatched")
	}
	p.Update(cmd())
	if p.working || len(p.rules.Shell) != 1 {
		t.Fatalf("guard not loaded: %s", p.View())
	}
	return p, reader
}

func TestV2GuardPatternUsesActualMatcherWithoutExecuting(t *testing.T) {
	p, _ := loadGuardFixture(t)
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	// If this were executed, the shell expansion would create a real marker.
	marker := filepath.Join(t.TempDir(), "executed")
	sample := "rm -rf /; touch " + marker
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(sample), Paste: true})
	if _, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("matcher dispatched a side effect")
	}
	if !strings.Contains(p.status, "Matches selected pattern: block") || !strings.Contains(p.status, "command not executed") {
		t.Fatalf("real matcher not explained: %s", p.status)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("sample was executed")
	}
	p.beginTest()
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("echo safe"), Paste: true})
	p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(p.status, "No match") {
		t.Fatal(p.status)
	}
	p.beginTest()
	p.testField.SetValue("界")
	p.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if !utf8.ValidString(p.testField.Value()) || p.testField.Value() != "" {
		t.Fatal("backspace corrupted UTF-8")
	}
	p.testField.SetValue("keep")
	for _, paste := range []string{strings.Repeat("x", 65537), "echo safe\nrm -rf /", "\x1b[2J"} {
		p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(paste), Paste: true})
		if p.testField.Value() != "keep" {
			t.Fatal("invalid sample paste changed input")
		}
	}
	p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.OwnsEscape() {
		t.Fatal("Escape did not close pattern test")
	}
}

func TestV2GuardDoesNotAdvertiseUnconnectedIngestionMutations(t *testing.T) {
	p, reader := loadGuardFixture(t)
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyTab})
	if cmd == nil {
		t.Fatal("section query not dispatched")
	}
	p.Update(cmd())
	for _, r := range "ensady " {
		if _, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}); cmd != nil {
			t.Fatal("read-only key dispatched a mutation")
		}
	}
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if p.testMode || !strings.Contains(p.status, "until G2") {
		t.Fatal("ingestion presented as enforced or editable")
	}
	view := p.View()
	for _, label := range []string{"Save", "Activate", "Discard", "[e]", "[n]", "[d]"} {
		if strings.Contains(view, label) {
			t.Fatalf("unsupported action advertised: %s", label)
		}
	}
	p.Update(tea.KeyMsg{Type: tea.KeyTab})
	if reader.reads != 2 || p.section != config.GuardShell {
		t.Fatal("section navigation reloaded cached source")
	}
}

func TestV2GuardFailuresRetainObservationAndRejectForeignReplies(t *testing.T) {
	p, reader := loadGuardFixture(t)
	p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	reader.err = errors.New("permission denied")
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	result := cmd().(guardResultMsg)
	p.Update(result)
	if !strings.Contains(p.View(), "delete") || !strings.Contains(p.status, "permission denied") || !p.OwnsEscape() {
		t.Fatal("failed read erased prior inspection")
	}
	other := NewSecurityGuardPane(t.Context(), reader, false)
	other.Update(result)
	if len(other.snapshots) != 0 || other.status != "" {
		t.Fatal("old owner reply crossed workspace lifetime")
	}
	result.epoch--
	p.status = "unchanged"
	p.Update(result)
	if p.status != "unchanged" {
		t.Fatal("stale reply was applied")
	}
	for _, size := range [][2]int{{20, 7}, {80, 24}, {120, 40}} {
		p.SetSize(size[0], size[1])
		lines := strings.Split(ansi.Strip(p.View()), "\n")
		if len(lines) > size[1] {
			t.Fatal("inspector overflowed viewport")
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("inspector overflowed width")
			}
		}
	}
}

func TestV2GuardRootRoutingPreservesComposerWithoutExecutionRuntime(t *testing.T) {
	reader := &guardReaderFixture{}
	m := NewRootModel(t.Context(), HostOptions{Guards: reader})
	m.inputBar.SetValue("unfinished Euclo request")
	_, cmd := rootHandleSecurityGuard(&m, nil)
	if cmd == nil || m.activeTab != TabSecurityGuard || !m.focus.State().InRegion1() {
		t.Fatal("guard could not open without execution runtime")
	}
	next, _ := m.Update(cmd())
	m = next.(RootModel)
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyEsc}, {Type: tea.KeyRunes, Runes: []rune{'t'}}, {Type: tea.KeyRunes, Runes: []rune("echo harmless"), Paste: true}, {Type: tea.KeyEnter}, {Type: tea.KeyEsc}} {
		next, _ = m.Update(key)
		m = next.(RootModel)
	}
	if m.inputBar.Value() != "unfinished Euclo request" || !m.focus.State().InInput() {
		t.Fatal("guard keys corrupted composer or Escape failed")
	}
	if !strings.Contains(m.guard.status, "No match") {
		t.Fatal("root did not route pattern input")
	}
	entries := m.settingsEntries()
	found := false
	for _, entry := range entries {
		if entry.command == "securityguard" {
			found = true
		}
	}
	if !found {
		t.Fatal("guard missing from Settings before runtime startup")
	}
}

func TestV2GuardActualDiskInspectionCannotMutatePolicy(t *testing.T) {
	workspace := t.TempDir()
	securityDir := filepath.Join(workspace, "relurpify_cfg", "security")
	if err := os.MkdirAll(securityDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(securityDir, "shell.policy.yaml")
	if err := os.WriteFile(path, []byte(guardShellFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	m := NewRootModel(t.Context(), HostOptions{Workspace: workspace})
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 96, Height: 24})
	m = resized.(RootModel)
	m.inputBar.SetValue("retain")
	_, cmd := rootHandleSecurityGuard(&m, nil)
	if cmd == nil {
		t.Fatal("disk inspection not dispatched")
	}
	next, _ := m.Update(cmd())
	m = next.(RootModel)
	if !strings.Contains(m.guard.View(), "delete") {
		t.Fatalf("disk rule not visible: %s", m.guard.View())
	}
	// Historical write chords and a full paste are inert in inspection mode.
	for _, key := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune{'e'}}, {Type: tea.KeyRunes, Runes: []rune{'n'}}, {Type: tea.KeyRunes, Runes: []rune{'d'}}, {Type: tea.KeySpace}, {Type: tea.KeyRunes, Runes: []rune{'s'}}, {Type: tea.KeyRunes, Runes: []rune{'a'}}, {Type: tea.KeyRunes, Runes: []rune("schema: changed"), Paste: true}} {
		next, cmd = m.Update(key)
		m = next.(RootModel)
		if cmd != nil {
			t.Fatal("read-only interaction dispatched an effect")
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != guardShellFixture {
		t.Fatalf("inspection mutated disk: %q %v", data, err)
	}
	entries, err := os.ReadDir(securityDir)
	if err != nil || len(entries) != 1 {
		t.Fatal("inspection created files")
	}
	before := m.activeTab
	_, cmd = rootHandleSecurityGuard(&m, []string{"save"})
	if cmd != nil || m.activeTab != before {
		t.Fatal("unsupported guard subcommand was dispatched")
	}
}

func TestV2GuardPatternFieldKeepsGlobalAttentionReachable(t *testing.T) {
	m := NewRootModel(t.Context(), HostOptions{Guards: &guardReaderFixture{}})
	m.notifQ.Push(NotificationItem{ID: "hitl-1", Kind: NotifKindHITL, Msg: "execution approval: euclo.policy.gate"})
	m.inputBar.SetValue("unsent request")
	_, cmd := rootHandleSecurityGuard(&m, nil)
	next, _ := m.Update(cmd())
	m = next.(RootModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = next.(RootModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("retained sample"), Paste: true})
	m = next.(RootModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	m = next.(RootModel)
	if !m.decisionFocused || !m.guard.testMode || m.guard.testField.Value() != "retained sample" || m.inputBar.Value() != "unsent request" {
		t.Fatal("guard swallowed global attention or lost retained inputs")
	}
}
