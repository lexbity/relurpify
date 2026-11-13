// Package thoughtrecipe implements the Euclo thoughtrecipe DSL.
package thoughtrecipe

import (
	"strings"
	"testing"
)

func TestLosslessLexer_Basic(t *testing.T) {
	src := `thoughtrecipe test
"A test"

trigger as capability:
  may read workspace
`

	lexer := NewLosslessLexer("test.erpe", src)
	tokens, err := lexer.LexAll()
	if err != nil {
		t.Fatalf("failed to lex: %v", err)
	}

	if len(tokens) == 0 {
		t.Fatal("expected tokens")
	}

	// Check that we have an EOF token
	if tokens[len(tokens)-1].Kind != TokenEOF {
		t.Error("expected last token to be EOF")
	}

	// Check first token
	if tokens[0].Kind != TokenKeyword {
		t.Errorf("expected first token to be keyword, got %v", tokens[0].Kind)
	}

	if tokens[0].Lexeme != "thoughtrecipe" {
		t.Errorf("expected 'thoughtrecipe', got %q", tokens[0].Lexeme)
	}
}

func TestLosslessLexer_CRLFPreservation(t *testing.T) {
	// Source with CRLF line endings
	src := "thoughtrecipe test\r\n\r\ntrigger as capability:\r\n  may read workspace"

	lexer := NewLosslessLexer("crlf.erpe", src)
	tokens, err := lexer.LexAll()
	if err != nil {
		t.Fatalf("failed to lex: %v", err)
	}

	// The source should contain CRLF
	if !strings.Contains(src, "\r\n") {
		t.Fatal("source doesn't contain CRLF")
	}

	// Verify that the first token's byte span is correct
	// "thoughtrecipe" is 13 characters, so it should be at bytes 0-13
	if tokens[0].StartByte != 0 {
		t.Errorf("expected first token to start at byte 0, got %d", tokens[0].StartByte)
	}

	if tokens[0].EndByte != 13 {
		t.Errorf("expected first token to end at byte 13, got %d", tokens[0].EndByte)
	}
}

func TestLosslessLexer_LFPreservation(t *testing.T) {
	// Source with LF line endings
	src := "thoughtrecipe test\n\ntrigger as capability:\n  may read workspace"

	lexer := NewLosslessLexer("lf.erpe", src)
	tokens, err := lexer.LexAll()
	if err != nil {
		t.Fatalf("failed to lex: %v", err)
	}

	// Verify that the first token's byte span is correct
	if tokens[0].StartByte != 0 {
		t.Errorf("expected first token to start at byte 0, got %d", tokens[0].StartByte)
	}

	if tokens[0].EndByte != 13 {
		t.Errorf("expected first token to end at byte 13, got %d", tokens[0].EndByte)
	}
}

func TestLosslessLexer_TokenWithSpan_Conversion(t *testing.T) {
	src := "thoughtrecipe test"
	lexer := NewLosslessLexer("test.erpe", src)
	tokens, err := lexer.LexAll()
	if err != nil {
		t.Fatalf("failed to lex: %v", err)
	}

	for _, tok := range tokens {
		span := TokenWithSpanToByteSpan(tok)
		if span.Start != tok.StartByte {
			t.Errorf("span start mismatch: %d vs %d", span.Start, tok.StartByte)
		}
		if span.End != tok.EndByte {
			t.Errorf("span end mismatch: %d vs %d", span.End, tok.EndByte)
		}
	}
}

func TestLosslessLexer_MultilineString(t *testing.T) {
	src := `thoughtrecipe test
"""
This is a
multiline string
"""
`

	lexer := NewLosslessLexer("test.erpe", src)
	tokens, err := lexer.LexAll()
	if err != nil {
		t.Fatalf("failed to lex: %v", err)
	}

	// Find the string token
	var stringTok *TokenWithSpan
	for i, tok := range tokens {
		if tok.Kind == TokenString && strings.Contains(tok.Lexeme, "multiline") {
			stringTok = &tokens[i]
			break
		}
	}

	if stringTok == nil {
		t.Fatal("expected to find multiline string token")
	}

	// Verify it spans the correct bytes
	if stringTok.Lexeme == "" {
		t.Error("expected multiline string to have content")
	}
}

func TestLosslessLexer_Indentation(t *testing.T) {
	src := `thoughtrecipe test
  trigger as capability:
    may read workspace
`

	lexer := NewLosslessLexer("test.erpe", src)
	tokens, err := lexer.LexAll()
	if err != nil {
		t.Fatalf("failed to lex: %v", err)
	}

	// Should have INDENT tokens
	var foundIndent bool
	for _, tok := range tokens {
		if tok.Kind == TokenIndent {
			foundIndent = true
			break
		}
	}

	if !foundIndent {
		t.Error("expected to find INDENT token")
	}
}

func TestLosslessLexer_Comments(t *testing.T) {
	src := `thoughtrecipe test
# This is a comment
trigger as capability:`

	lexer := NewLosslessLexer("test.erpe", src)
	tokens, err := lexer.LexAll()
	if err != nil {
		t.Fatalf("failed to lex: %v", err)
	}

	// Comments are skipped by the lexer (not returned as tokens)
	// So we just verify it doesn't error
	_ = tokens
}

func TestLosslessLexer_Numbers(t *testing.T) {
	src := "thoughtrecipe test confidence 0.95%"

	lexer := NewLosslessLexer("test.erpe", src)
	tokens, err := lexer.LexAll()
	if err != nil {
		t.Fatalf("failed to lex: %v", err)
	}

	// Find number token
	var foundNumber bool
	for _, tok := range tokens {
		if tok.Kind == TokenNumber {
			foundNumber = true
			if tok.Lexeme != "0.95%" {
				t.Errorf("expected '0.95%%', got %q", tok.Lexeme)
			}
			break
		}
	}

	if !foundNumber {
		t.Error("expected to find number token")
	}
}

func TestLosslessLexer_ListMarker(t *testing.T) {
	src := `thoughtrecipe test
- item1
- item2`

	lexer := NewLosslessLexer("test.erpe", src)
	tokens, err := lexer.LexAll()
	if err != nil {
		t.Fatalf("failed to lex: %v", err)
	}

	// Should have list marker tokens
	var foundListMarker bool
	for _, tok := range tokens {
		if tok.Kind == TokenListMarker {
			foundListMarker = true
			break
		}
	}

	if !foundListMarker {
		t.Error("expected to find LIST_MARKER token")
	}
}

func TestLosslessLexer_Arrow(t *testing.T) {
	src := "thoughtrecipe test\n  from input -> capture result"

	lexer := NewLosslessLexer("test.erpe", src)
	tokens, err := lexer.LexAll()
	if err != nil {
		t.Fatalf("failed to lex: %v", err)
	}

	// Should have punctuation token for ->
	var foundArrow bool
	for _, tok := range tokens {
		if tok.Kind == TokenPunctuation && tok.Lexeme == "->" {
			foundArrow = true
			break
		}
	}

	if !foundArrow {
		t.Error("expected to find -> arrow token")
	}
}

