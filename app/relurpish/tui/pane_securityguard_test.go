package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	tea "github.com/charmbracelet/bubbletea"
)

func TestV2GuardPatternUsesActualMatcherWithoutExecuting(t *testing.T) {
	p := &SecurityGuardPane{th: theme.Default(), shellRules: []sandbox.BlacklistRule{{ID: "destructive-root", Raw: `rm\s+-rf\s+/`, Action: sandbox.BlacklistActionBlock}}}
	p.SetSize(80, 19)
	p.beginTest()
	if !p.OwnsEscape() {
		t.Fatal("pattern testing did not own its focus frame")
	}
	for _, msg := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("rm -rf /"), Paste: true}, {Type: tea.KeyEnter}} {
		if _, cmd := p.Update(msg); cmd != nil {
			t.Fatal("pattern testing dispatched an operation")
		}
	}
	if !strings.Contains(p.testResult, "Matches selected rule: block") || !strings.Contains(p.testResult, "command not executed") {
		t.Fatalf("actual matcher was not explained: %s", p.testResult)
	}
	p.beginTest()
	p.testBuffer = "界"
	p.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if !utf8.ValidString(p.testBuffer) || p.testBuffer != "" {
		t.Fatal("field backspace corrupted UTF-8")
	}
	p.testBuffer = "keep"
	p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strings.Repeat("x", 65537)), Paste: true})
	if p.testBuffer != "keep" {
		t.Fatal("oversized pattern-test paste changed the field")
	}
	p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.OwnsEscape() {
		t.Fatal("Escape did not close pattern testing")
	}
}

func TestV2GuardDoesNotAdvertiseUnconnectedIngestionMutations(t *testing.T) {
	p := &SecurityGuardPane{th: theme.Default(), activePanel: securityPanelIngestion}
	p.SetSize(80, 19)
	for _, r := range "endt " {
		if _, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}); cmd != nil {
			t.Fatal("unconnected ingestion policy dispatched an effect")
		}
	}
	if p.editing || p.testMode || p.confirmDrop || !strings.Contains(p.View(), "owner is not connected") {
		t.Fatal("unconnected ingestion policy was presented as editable")
	}
	if strings.Contains(p.View(), "no guardrails configured") {
		t.Fatal("missing inspection was presented as an empty policy")
	}
}
