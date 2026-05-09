package thoughtrecipe

import (
	"strings"
	"testing"
)

func TestLexerTokenizesKeywordsIdentifiersPunctuationAndIndentation(t *testing.T) {
	src := "thoughtrecipe code_review\ntrigger as capability:\n  may read workspace\n"
	tokens := mustLex(t, "thoughtrecipe.euclo", src)

	want := []Token{
		{Kind: TokenKeyword, Lexeme: "thoughtrecipe", File: "thoughtrecipe.euclo", Line: 1, Column: 1},
		{Kind: TokenIdentifier, Lexeme: "code_review", File: "thoughtrecipe.euclo", Line: 1, Column: 15},
		{Kind: TokenKeyword, Lexeme: "trigger", File: "thoughtrecipe.euclo", Line: 2, Column: 1},
		{Kind: TokenKeyword, Lexeme: "as", File: "thoughtrecipe.euclo", Line: 2, Column: 9},
		{Kind: TokenIdentifier, Lexeme: "capability", File: "thoughtrecipe.euclo", Line: 2, Column: 12},
		{Kind: TokenPunctuation, Lexeme: ":", File: "thoughtrecipe.euclo", Line: 2, Column: 22},
		{Kind: TokenIndent, Lexeme: "  ", File: "thoughtrecipe.euclo", Line: 3, Column: 1},
		{Kind: TokenKeyword, Lexeme: "may", File: "thoughtrecipe.euclo", Line: 3, Column: 3},
		{Kind: TokenKeyword, Lexeme: "read", File: "thoughtrecipe.euclo", Line: 3, Column: 7},
		{Kind: TokenIdentifier, Lexeme: "workspace", File: "thoughtrecipe.euclo", Line: 3, Column: 12},
		{Kind: TokenDedent, Lexeme: "", File: "thoughtrecipe.euclo", Line: 4, Column: 1},
		{Kind: TokenEOF, Lexeme: "", File: "thoughtrecipe.euclo", Line: 4, Column: 1},
	}

	assertTokens(t, tokens, want)
}

func TestLexerTokenizesListMarkers(t *testing.T) {
	src := "choices:\n  - review\n  - refactor\n"
	tokens := mustLex(t, "choices.euclo", src)

	want := []Token{
		{Kind: TokenKeyword, Lexeme: "choices", Line: 1, Column: 1},
		{Kind: TokenPunctuation, Lexeme: ":", Line: 1, Column: 8},
		{Kind: TokenIndent, Lexeme: "  ", Line: 2, Column: 1},
		{Kind: TokenListMarker, Lexeme: "-", Line: 2, Column: 3},
		{Kind: TokenKeyword, Lexeme: "review", Line: 2, Column: 5},
		{Kind: TokenListMarker, Lexeme: "-", Line: 3, Column: 3},
		{Kind: TokenIdentifier, Lexeme: "refactor", Line: 3, Column: 5},
		{Kind: TokenDedent, Lexeme: "", Line: 4, Column: 1},
		{Kind: TokenEOF, Lexeme: "", Line: 4, Column: 1},
	}

	assertTokens(t, tokens, want)
}

func TestLexerTokenizesMultilineString(t *testing.T) {
	src := "goal \"\"\"\nReview the codebase.\n- correctness issues\n\"\"\"\n"
	tokens := mustLex(t, "string.euclo", src)

	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(tokens))
	}
	if tokens[0].Kind != TokenKeyword || tokens[0].Lexeme != "goal" {
		t.Fatalf("unexpected first token: %+v", tokens[0])
	}
	if tokens[1].Kind != TokenString {
		t.Fatalf("unexpected string token kind: %+v", tokens[1])
	}
	if tokens[1].Line != 1 || tokens[1].Column != 6 {
		t.Fatalf("string token location = %d:%d, want 1:6", tokens[1].Line, tokens[1].Column)
	}
	if !strings.Contains(tokens[1].Lexeme, "correctness issues") {
		t.Fatalf("string token lexeme did not preserve multiline body: %q", tokens[1].Lexeme)
	}
	if tokens[2].Kind != TokenEOF {
		t.Fatalf("unexpected final token: %+v", tokens[2])
	}
}

func TestLexerStripsComments(t *testing.T) {
	src := "thoughtrecipe demo # header comment\n# full-line comment\ninput prompt: user.request # trailing\n"
	tokens := mustLex(t, "comments.euclo", src)

	want := []Token{
		{Kind: TokenKeyword, Lexeme: "thoughtrecipe", File: "comments.euclo", Line: 1, Column: 1},
		{Kind: TokenIdentifier, Lexeme: "demo", File: "comments.euclo", Line: 1, Column: 15},
		{Kind: TokenKeyword, Lexeme: "input", File: "comments.euclo", Line: 3, Column: 1},
		{Kind: TokenKeyword, Lexeme: "prompt", File: "comments.euclo", Line: 3, Column: 7},
		{Kind: TokenPunctuation, Lexeme: ":", File: "comments.euclo", Line: 3, Column: 13},
		{Kind: TokenKeyword, Lexeme: "user", File: "comments.euclo", Line: 3, Column: 15},
		{Kind: TokenPunctuation, Lexeme: ".", File: "comments.euclo", Line: 3, Column: 19},
		{Kind: TokenIdentifier, Lexeme: "request", File: "comments.euclo", Line: 3, Column: 20},
		{Kind: TokenEOF, Lexeme: "", File: "comments.euclo", Line: 4, Column: 1},
	}

	assertTokens(t, tokens, want)
}

func TestLexerTokenizesCaptureArrow(t *testing.T) {
	src := "capture:\n  summary: Markdown -> output.result\n"
	tokens := mustLex(t, "capture.euclo", src)

	want := []Token{
		{Kind: TokenKeyword, Lexeme: "capture", Line: 1, Column: 1},
		{Kind: TokenPunctuation, Lexeme: ":", Line: 1, Column: 8},
		{Kind: TokenIndent, Lexeme: "  ", Line: 2, Column: 1},
		{Kind: TokenIdentifier, Lexeme: "summary", Line: 2, Column: 3},
		{Kind: TokenPunctuation, Lexeme: ":", Line: 2, Column: 10},
		{Kind: TokenIdentifier, Lexeme: "Markdown", Line: 2, Column: 12},
		{Kind: TokenPunctuation, Lexeme: "->", Line: 2, Column: 21},
		{Kind: TokenIdentifier, Lexeme: "output", Line: 2, Column: 24},
		{Kind: TokenPunctuation, Lexeme: ".", Line: 2, Column: 30},
		{Kind: TokenIdentifier, Lexeme: "result", Line: 2, Column: 31},
		{Kind: TokenDedent, Lexeme: "", Line: 3, Column: 1},
		{Kind: TokenEOF, Lexeme: "", Line: 3, Column: 1},
	}

	assertTokens(t, tokens, want)
}

func TestLexerRejectsMalformedIndentation(t *testing.T) {
	loader := NewLexer("bad.euclo", "thoughtrecipe demo\n    trigger as capability\n")
	_, err := loader.LexAll()
	if err == nil {
		t.Fatal("expected indentation error")
	}
	if !strings.Contains(err.Error(), "indentation") {
		t.Fatalf("expected indentation error, got %v", err)
	}
}

func TestLexerRejectsUnterminatedString(t *testing.T) {
	loader := NewLexer("bad.euclo", "goal \"unterminated\n")
	_, err := loader.LexAll()
	if err == nil {
		t.Fatal("expected string error")
	}
	if !strings.Contains(err.Error(), "unterminated string") {
		t.Fatalf("expected unterminated string error, got %v", err)
	}
}

func mustLex(t *testing.T, filename, src string) []Token {
	t.Helper()
	tokens, err := NewLexer(filename, src).LexAll()
	if err != nil {
		t.Fatalf("LexAll failed: %v", err)
	}
	return tokens
}

func assertTokens(t *testing.T, got, want []Token) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("token count = %d, want %d\n got: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Kind != want[i].Kind || got[i].Lexeme != want[i].Lexeme || got[i].Line != want[i].Line || got[i].Column != want[i].Column {
			t.Fatalf("token %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}
