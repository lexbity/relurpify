// Package thoughtrecipe implements the Euclo thoughtrecipe DSL.
package thoughtrecipe

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// LosslessLexer tokenizes Euclo thoughtrecipe source while preserving
// all original bytes including line endings (CRLF vs LF).
// This is the P04-compliant lexer that enables lossless source editing.
type LosslessLexer struct {
	filename string
	src      string
	pos      int  // current position in bytes
	line     int  // current line number (1-indexed)
	column   int  // current column number (1-indexed)
	startPos int  // start position of current token in bytes
	startLine int // start line of current token
	startCol  int // start column of current token
	atLineStart bool
	lineHasToken bool
	indentStack []int
	pending []TokenWithSpan
	done    bool
	// byteOffset tracks the absolute byte offset for span calculation
	byteOffset int
}

// TokenWithSpan extends Token with byte-based span information.
// This enables lossless editing by tracking exact byte offsets.
type TokenWithSpan struct {
	Token
	// StartByte is the inclusive start byte offset (0-indexed).
	StartByte int
	// EndByte is the exclusive end byte offset (0-indexed).
	EndByte int
}

// NewLosslessLexer creates a lexer that preserves all original bytes.
// Unlike NewLexer, this does NOT normalize CRLF to LF.
func NewLosslessLexer(filename, src string) *LosslessLexer {
	return &LosslessLexer{
		filename:    filename,
		src:         src,
		line:        1,
		column:      1,
		atLineStart: true,
		indentStack: []int{0},
		byteOffset:  0,
	}
}

// LexAll tokenizes the complete source, preserving exact byte spans.
func (l *LosslessLexer) LexAll() ([]TokenWithSpan, error) {
	var tokens []TokenWithSpan
	for {
		tok, err := l.Next()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
		if tok.Kind == TokenEOF {
			return tokens, nil
		}
	}
}

// Next returns the next token with byte span information.
func (l *LosslessLexer) Next() (TokenWithSpan, error) {
	if len(l.pending) > 0 {
		tok := l.pending[0]
		l.pending = l.pending[1:]
		return tok, nil
	}
	if l.done {
		return TokenWithSpan{
			Token:    l.token(TokenEOF, "", l.line, l.column),
			StartByte: l.byteOffset,
			EndByte:   l.byteOffset,
		}, nil
	}

	for {
		if l.atLineStart {
			if err := l.beginLine(); err != nil {
				return TokenWithSpan{}, err
			}
			if len(l.pending) > 0 {
				tok := l.pending[0]
				l.pending = l.pending[1:]
				return tok, nil
			}
			if l.done {
				return TokenWithSpan{
					Token:    l.token(TokenEOF, "", l.line, l.column),
					StartByte: l.byteOffset,
					EndByte:   l.byteOffset,
				}, nil
			}
			continue
		}

		if l.pos >= len(l.src) {
			return l.finishEOF()
		}

		startPos := l.pos
		startLine := l.line
		startCol := l.column
		startByte := l.byteOffset

		r, _ := l.peek()
		if r == '\n' {
			l.consume()
			l.atLineStart = true
			l.lineHasToken = false
			continue
		}
		if r == '\r' {
			// Handle CRLF: if followed by LF, consume both as a single newline
			// but track the bytes accurately
			if l.pos+1 < len(l.src) && l.src[l.pos+1] == '\n' {
				l.consume() // consume CR
				l.consume() // consume LF
				l.atLineStart = true
				l.lineHasToken = false
				continue
			}
			// Standalone CR
			l.consume()
			l.atLineStart = true
			l.lineHasToken = false
			continue
		}
		if isSpace(r) {
			l.consume()
			continue
		}
		if r == '#' {
			l.skipLineComment()
			continue
		}

		switch {
		case r == '"':
			tok, err := l.scanString(startLine, startCol, startPos)
			if err != nil {
				return TokenWithSpan{}, err
			}
			l.lineHasToken = true
			return TokenWithSpan{
				Token:     tok,
				StartByte: startByte,
				EndByte:   l.byteOffset,
			}, nil
		case isIdentStart(r):
			tok := l.scanIdentifier(startLine, startCol, startPos)
			l.lineHasToken = true
			return TokenWithSpan{
				Token:     tok,
				StartByte: startByte,
				EndByte:   l.byteOffset,
			}, nil
		case isDigit(r):
			tok := l.scanNumber(startLine, startCol, startPos)
			l.lineHasToken = true
			return TokenWithSpan{
				Token:     tok,
				StartByte: startByte,
				EndByte:   l.byteOffset,
			}, nil
		case r == '-' && l.peekArrow():
			l.consume()
			l.consume()
			tok := l.token(TokenPunctuation, "->", startLine, startCol)
			l.lineHasToken = true
			return TokenWithSpan{
				Token:     tok,
				StartByte: startByte,
				EndByte:   l.byteOffset,
			}, nil
		case r == '-' && !l.lineHasToken && l.isListMarkerAhead():
			l.consume()
			tok := l.token(TokenListMarker, "-", startLine, startCol)
			l.lineHasToken = true
			return TokenWithSpan{
				Token:     tok,
				StartByte: startByte,
				EndByte:   l.byteOffset,
			}, nil
		case isPunctuation(r):
			l.consume()
			tok := l.token(TokenPunctuation, string(r), startLine, startCol)
			l.lineHasToken = true
			return TokenWithSpan{
				Token:     tok,
				StartByte: startByte,
				EndByte:   l.byteOffset,
			}, nil
		default:
			return TokenWithSpan{}, fmt.Errorf("%s:%d:%d: unexpected character %q",
				l.filename, l.line, l.column, r)
		}
	}
}

func (l *LosslessLexer) beginLine() error {
	if l.pos >= len(l.src) {
		for len(l.indentStack) > 1 {
			l.indentStack = l.indentStack[:len(l.indentStack)-1]
			l.pending = append(l.pending, TokenWithSpan{
				Token:     l.token(TokenDedent, "", l.line, l.column),
				StartByte: l.byteOffset,
				EndByte:   l.byteOffset,
			})
		}
		l.done = true
		return nil
	}

	startLine, startCol := l.line, l.column
	startByte := l.byteOffset
	spaces := 0
	for {
		r, _ := l.peek()
		switch r {
		case ' ':
			spaces++
			l.consume()
		case '\t':
			return fmt.Errorf("%s:%d:%d: tabs are not valid indentation",
				l.filename, l.line, l.column)
		default:
			goto done
		}
	}
done:
	r, _ := l.peek()
	if r == '\n' {
		l.consume()
		l.atLineStart = true
		return nil
	}
	if r == '\r' {
		// Handle CRLF
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == '\n' {
			l.consume()
			l.consume()
			l.atLineStart = true
			return nil
		}
		l.consume()
		l.atLineStart = true
		return nil
	}
	if r == '#' {
		l.skipLineComment()
		return nil
	}
	top := l.indentStack[len(l.indentStack)-1]
	if spaces > top {
		if spaces != top+2 {
			return fmt.Errorf("%s:%d:%d: indentation must increase by two spaces",
				l.filename, startLine, startCol)
		}
		l.indentStack = append(l.indentStack, spaces)
		l.pending = append(l.pending, TokenWithSpan{
			Token:     l.token(TokenIndent, strings.Repeat(" ", spaces), startLine, startCol),
			StartByte: startByte,
			EndByte:   l.byteOffset,
		})
	} else if spaces < top {
		for len(l.indentStack) > 1 && l.indentStack[len(l.indentStack)-1] > spaces {
			l.indentStack = l.indentStack[:len(l.indentStack)-1]
			l.pending = append(l.pending, TokenWithSpan{
				Token:     l.token(TokenDedent, "", startLine, startCol),
				StartByte: startByte,
				EndByte:   l.byteOffset,
			})
		}
		if l.indentStack[len(l.indentStack)-1] != spaces {
			return fmt.Errorf("%s:%d:%d: indentation does not match any open block",
				l.filename, startLine, startCol)
		}
	}

	l.atLineStart = false
	l.lineHasToken = false
	return nil
}

func (l *LosslessLexer) finishEOF() (TokenWithSpan, error) {
	startByte := l.byteOffset
	for len(l.indentStack) > 1 {
		l.indentStack = l.indentStack[:len(l.indentStack)-1]
		l.pending = append(l.pending, TokenWithSpan{
			Token:     l.token(TokenDedent, "", l.line, l.column),
			StartByte: l.byteOffset,
			EndByte:   l.byteOffset,
		})
	}
	l.done = true
	if len(l.pending) > 0 {
		tok := l.pending[0]
		l.pending = l.pending[1:]
		return tok, nil
	}
	return TokenWithSpan{
		Token:    l.token(TokenEOF, "", l.line, l.column),
		StartByte: startByte,
		EndByte:   startByte,
	}, nil
}

func (l *LosslessLexer) skipLineComment() {
	for {
		if l.pos >= len(l.src) {
			l.atLineStart = true
			return
		}
		r, _ := l.peek()
		l.consume()
		if r == '\n' {
			l.atLineStart = true
			l.lineHasToken = false
			return
		}
		if r == '\r' {
			// Check for CRLF
			if l.pos < len(l.src) && l.src[l.pos] == '\n' {
				l.consume()
			}
			l.atLineStart = true
			l.lineHasToken = false
			return
		}
	}
}

func (l *LosslessLexer) scanIdentifier(line, col, startPos int) Token {
	for {
		r, _ := l.peek()
		if !isIdentPart(r) {
			break
		}
		l.consume()
	}
	lexeme := l.src[startPos:l.pos]
	kind := TokenIdentifier
	if _, ok := reservedKeywords[lexeme]; ok {
		kind = TokenKeyword
	}
	return l.token(kind, lexeme, line, col)
}

func (l *LosslessLexer) scanString(line, col, startPos int) (Token, error) {
	if l.matchStringPrefix(`"""`) {
		l.consume()
		l.consume()
		l.consume()
		for {
			if l.pos >= len(l.src) {
				return Token{}, fmt.Errorf("%s:%d:%d: unterminated multiline string",
					l.filename, line, col)
			}
			if l.matchStringPrefix(`"""`) {
				l.consume()
				l.consume()
				l.consume()
				break
			}
			l.consume()
		}
		return l.token(TokenString, l.src[startPos:l.pos], line, col), nil
	}

	l.consume() // opening quote
	for {
		if l.pos >= len(l.src) {
			return Token{}, fmt.Errorf("%s:%d:%d: unterminated string",
				l.filename, line, col)
		}
		r, _ := l.peek()
		if r == '\n' {
			return Token{}, fmt.Errorf("%s:%d:%d: unterminated string",
				l.filename, line, col)
		}
		if r == '\\' {
			l.consume()
			if l.pos >= len(l.src) {
				return Token{}, fmt.Errorf("%s:%d:%d: unterminated string escape",
					l.filename, line, col)
			}
			l.consume()
			continue
		}
		l.consume()
		if r == '"' {
			break
		}
	}

	return l.token(TokenString, l.src[startPos:l.pos], line, col), nil
}

func (l *LosslessLexer) scanNumber(line, col, startPos int) Token {
	hasDot := false
	for {
		r, _ := l.peek()
		switch {
		case isDigit(r):
			l.consume()
		case r == '.' && !hasDot:
			hasDot = true
			l.consume()
		case r == '%':
			l.consume()
			return l.token(TokenNumber, l.src[startPos:l.pos], line, col)
		default:
			return l.token(TokenNumber, l.src[startPos:l.pos], line, col)
		}
	}
}

func (l *LosslessLexer) isListMarkerAhead() bool {
	if l.pos >= len(l.src) {
		return true
	}
	r, _ := l.peekN(1)
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '#'
}

func (l *LosslessLexer) peekArrow() bool {
	if l.pos+1 >= len(l.src) {
		return false
	}
	return l.src[l.pos+1] == '>'
}

func (l *LosslessLexer) matchStringPrefix(prefix string) bool {
	return strings.HasPrefix(l.src[l.pos:], prefix)
}

func (l *LosslessLexer) peek() (rune, int) {
	if l.pos >= len(l.src) {
		return utf8.RuneError, 0
	}
	return utf8.DecodeRuneInString(l.src[l.pos:])
}

func (l *LosslessLexer) peekN(n int) (rune, int) {
	i := l.pos
	var r rune
	var width int
	for count := 0; count <= n; count++ {
		if i >= len(l.src) {
			return utf8.RuneError, 0
		}
		r, width = utf8.DecodeRuneInString(l.src[i:])
		if count == n {
			return r, width
		}
		i += width
	}
	return utf8.RuneError, 0
}

func (l *LosslessLexer) consume() rune {
	if l.pos >= len(l.src) {
		return utf8.RuneError
	}
	r, width := utf8.DecodeRuneInString(l.src[l.pos:])
	l.pos += width
	l.byteOffset += width
	if r == '\n' {
		l.line++
		l.column = 1
		l.atLineStart = true
		l.lineHasToken = false
		return r
	}
	if r == '\r' {
		// For CR, we still count it as a line increment
		// If followed by LF, the LF will also increment
		l.line++
		l.column = 1
		l.atLineStart = true
		l.lineHasToken = false
		return r
	}
	l.column++
	return r
}

func (l *LosslessLexer) token(kind TokenKind, lexeme string, line, col int) Token {
	return Token{
		Kind:   kind,
		Lexeme: lexeme,
		File:   l.filename,
		Line:   line,
		Column: col,
	}
}

// TokenWithSpanToByteSpan converts a token with span to a ByteSpan.
func TokenWithSpanToByteSpan(tok TokenWithSpan) ByteSpan {
	return ByteSpan{
		Start: tok.StartByte,
		End:   tok.EndByte,
	}
}

// ParseSourceWithLosslessLexer parses source using the lossless lexer.
// This preserves all original bytes including line endings.
func ParseSourceWithLosslessLexer(filename, src string) (*Document, []TokenWithSpan, error) {
	tokens, err := NewLosslessLexer(filename, src).LexAll()
	if err != nil {
		return nil, nil, err
	}
	
	// Convert TokenWithSpan to Token for the existing parser
	plainTokens := make([]Token, len(tokens))
	for i, tok := range tokens {
		plainTokens[i] = tok.Token
	}
	
	doc, err := NewParser(filename, plainTokens).ParseDocument()
	if err != nil {
		return nil, nil, err
	}
	
	return doc, tokens, nil
}
