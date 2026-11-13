package tui

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

func TestV2CommandArguments(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{`workspace "/tmp/a b"`, []string{"workspace", "/tmp/a b"}},
		{`ask --text "/literal\nsecond\tline"`, []string{"ask", "--text", "/literal\nsecond\tline"}},
		{`export json ""`, []string{"export", "json", ""}},
		{`ask --text "$(touch file) ` + "`command`" + `"`, []string{"ask", "--text", "$(touch file) `command`"}},
	} {
		got, err := commandWords(tc.input)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%q: got %q, %v; want %q", tc.input, got, err, tc.want)
		}
	}
	for _, input := range []string{`ask "unterminated`, `ask "x"tail`, `ask a"b`, `ask "\q"`, "ask \"a\n\""} {
		bar := NewInputBar()
		bar.SetValue("/" + input)
		_, cmd := bar.Update(tea.KeyMsg{Type: tea.KeyEnter}, TabChat)
		if cmd == nil {
			t.Fatalf("%q: missing argument diagnostic", input)
		}
		if _, rejected := cmd().(InputRejectedMsg); !rejected || bar.Value() != "/"+input {
			t.Fatalf("malformed command fell through to a request or lost its draft: %q", input)
		}
	}
}

func TestV2LiteralRequestRetainsBlockedDraft(t *testing.T) {
	m, _ := newFocusTestModel()
	m.startupLocked = true
	const draft = `/ask --text "/this is literal"`
	m.inputBar.SetValue(draft)
	updated, cmd := m.Update(CommandInvokedMsg{Name: "ask", Args: []string{"--text", "/this is literal"}, Draft: draft})
	m = updated.(RootModel)
	if cmd != nil || m.inputBar.Value() != draft {
		t.Fatal("blocked literal request dispatched or lost its draft")
	}
}

func FuzzCommandQuotedRoundTrip(f *testing.F) {
	for _, seed := range []string{"", "/literal", "界\nquoted \"text\"", "\\", "$(echo unsafe)", "\x00"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		if !utf8.ValidString(value) || len(value) > 65536 {
			return
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		words, err := commandWords("ask --text " + string(encoded))
		if err != nil || len(words) != 3 || words[2] != value {
			t.Fatalf("JSON literal did not round trip: %q %q %v", value, words, err)
		}
		if strings.ContainsAny(string(encoded), "\n\r") {
			t.Fatal("JSON encoder produced an unescaped line break")
		}
	})
}
