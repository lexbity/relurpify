package thoughtrecipe

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// TokenKind identifies a lexical token.
type TokenKind int

const (
	TokenIllegal TokenKind = iota
	TokenEOF
	TokenIndent
	TokenDedent
	TokenKeyword
	TokenIdentifier
	TokenString
	TokenNumber
	TokenPunctuation
	TokenListMarker
)

// String returns a stable name for the token kind.
func (k TokenKind) String() string {
	switch k {
	case TokenEOF:
		return "EOF"
	case TokenIndent:
		return "INDENT"
	case TokenDedent:
		return "DEDENT"
	case TokenKeyword:
		return "KEYWORD"
	case TokenIdentifier:
		return "IDENTIFIER"
	case TokenString:
		return "STRING"
	case TokenNumber:
		return "NUMBER"
	case TokenPunctuation:
		return "PUNCTUATION"
	case TokenListMarker:
		return "LIST_MARKER"
	default:
		return "ILLEGAL"
	}
}

// Token is a single lexical item with source location.
type Token struct {
	Kind   TokenKind
	Lexeme string
	File   string
	Line   int
	Column int
}

// Lexer tokenizes Euclo thoughtrecipe source.
type Lexer struct {
	filename     string
	src          string
	pos          int
	line         int
	column       int
	atLineStart  bool
	lineHasToken bool
	indentStack  []int
	pending      []Token
	done         bool
}

var reservedKeywords = map[string]struct{}{
	"thoughtrecipe": {},
	"import":        {},
	"trigger":       {},
	"as":            {},
	"may":           {},
	"invoke":        {},
	"family":        {},
	"keyword":       {},
	"handoff":       {},
	"read":          {},
	"write":         {},
	"input":         {},
	"type":          {},
	"agent":         {},
	"uses":          {},
	"run":           {},
	"from":          {},
	"goal":          {},
	"do":            {},
	"capture":       {},
	"route":         {},
	"when":          {},
	"otherwise":     {},
	"ask":           {},
	"user":          {},
	"question":      {},
	"choices":       {},
	"delegate":      {},
	"to":            {},
	"pipeline":      {},
	"stage":         {},
	"plan":          {},
	"step":          {},
	"verify":        {},
	"summarize":     {},
	"method":        {},
	"task":          {},
	"review":        {},
	"revise":        {},
	"source":        {},
	"link":          {},
	"prompt":        {},
	"until":         {},
	"synthesize":    {},
	"retry":         {},
	"attempts":      {},
	"missing":       {},
	"present":       {},
	"contains":      {},
	"confidence":    {},
	"below":         {},
	"is":            {},
	"on":            {},
	"with":          {},
}

// NewLexer creates a lexer for the provided source text.
func NewLexer(filename, src string) *Lexer {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	return &Lexer{
		filename:    filename,
		src:         src,
		line:        1,
		column:      1,
		atLineStart: true,
		indentStack: []int{0},
	}
}

// LexAll tokenizes the complete source.
func (l *Lexer) LexAll() ([]Token, error) {
	var tokens []Token
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

// Next returns the next token.
func (l *Lexer) Next() (Token, error) {
	if len(l.pending) > 0 {
		tok := l.pending[0]
		l.pending = l.pending[1:]
		return tok, nil
	}
	if l.done {
		return l.token(TokenEOF, "", l.line, l.column), nil
	}

	for {
		if l.atLineStart {
			if err := l.beginLine(); err != nil {
				return Token{}, err
			}
			if len(l.pending) > 0 {
				tok := l.pending[0]
				l.pending = l.pending[1:]
				return tok, nil
			}
			if l.done {
				return l.token(TokenEOF, "", l.line, l.column), nil
			}
			continue
		}

		if l.pos >= len(l.src) {
			return l.finishEOF()
		}

		r, _ := l.peek()
		if r == '\n' {
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

		startLine, startCol, startPos := l.line, l.column, l.pos
		switch {
		case r == '"':
			tok, err := l.scanString(startLine, startCol, startPos)
			if err != nil {
				return Token{}, err
			}
			l.lineHasToken = true
			return tok, nil
		case isIdentStart(r):
			tok := l.scanIdentifier(startLine, startCol, startPos)
			l.lineHasToken = true
			return tok, nil
		case isDigit(r):
			tok := l.scanNumber(startLine, startCol, startPos)
			l.lineHasToken = true
			return tok, nil
		case r == '-' && l.peekArrow():
			l.consume()
			l.consume()
			tok := l.token(TokenPunctuation, "->", startLine, startCol)
			l.lineHasToken = true
			return tok, nil
		case r == '-' && !l.lineHasToken && l.isListMarkerAhead():
			l.consume()
			tok := l.token(TokenListMarker, "-", startLine, startCol)
			l.lineHasToken = true
			return tok, nil
		case isPunctuation(r):
			l.consume()
			tok := l.token(TokenPunctuation, string(r), startLine, startCol)
			l.lineHasToken = true
			return tok, nil
		default:
			return Token{}, fmt.Errorf("%s:%d:%d: unexpected character %q", l.filename, l.line, l.column, r)
		}
	}
}

func (l *Lexer) beginLine() error {
	if l.pos >= len(l.src) {
		for len(l.indentStack) > 1 {
			l.indentStack = l.indentStack[:len(l.indentStack)-1]
			l.pending = append(l.pending, l.token(TokenDedent, "", l.line, l.column))
		}
		l.done = true
		return nil
	}

	startLine, startCol := l.line, l.column
	spaces := 0
	for {
		r, _ := l.peek()
		switch {
		case r == ' ':
			spaces++
			l.consume()
		case r == '\t':
			return fmt.Errorf("%s:%d:%d: tabs are not valid indentation", l.filename, l.line, l.column)
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
	if r == '#' {
		l.skipLineComment()
		return nil
	}
	top := l.indentStack[len(l.indentStack)-1]
	if spaces > top {
		if spaces != top+2 {
			return fmt.Errorf("%s:%d:%d: indentation must increase by two spaces", l.filename, startLine, startCol)
		}
		l.indentStack = append(l.indentStack, spaces)
		l.pending = append(l.pending, l.token(TokenIndent, strings.Repeat(" ", spaces), startLine, startCol))
	} else if spaces < top {
		for len(l.indentStack) > 1 && l.indentStack[len(l.indentStack)-1] > spaces {
			l.indentStack = l.indentStack[:len(l.indentStack)-1]
			l.pending = append(l.pending, l.token(TokenDedent, "", startLine, startCol))
		}
		if l.indentStack[len(l.indentStack)-1] != spaces {
			return fmt.Errorf("%s:%d:%d: indentation does not match any open block", l.filename, startLine, startCol)
		}
	}

	l.atLineStart = false
	l.lineHasToken = false
	return nil
}

func (l *Lexer) finishEOF() (Token, error) {
	for len(l.indentStack) > 1 {
		l.indentStack = l.indentStack[:len(l.indentStack)-1]
		l.pending = append(l.pending, l.token(TokenDedent, "", l.line, l.column))
	}
	l.done = true
	if len(l.pending) > 0 {
		tok := l.pending[0]
		l.pending = l.pending[1:]
		return tok, nil
	}
	return l.token(TokenEOF, "", l.line, l.column), nil
}

func (l *Lexer) skipLineComment() {
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
	}
}

func (l *Lexer) scanIdentifier(line, col, startPos int) Token {
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

func (l *Lexer) scanString(line, col, startPos int) (Token, error) {
	if l.matchStringPrefix(`"""`) {
		l.consume()
		l.consume()
		l.consume()
		for {
			if l.pos >= len(l.src) {
				return Token{}, fmt.Errorf("%s:%d:%d: unterminated multiline string", l.filename, line, col)
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
			return Token{}, fmt.Errorf("%s:%d:%d: unterminated string", l.filename, line, col)
		}
		r, _ := l.peek()
		if r == '\n' {
			return Token{}, fmt.Errorf("%s:%d:%d: unterminated string", l.filename, line, col)
		}
		if r == '\\' {
			l.consume()
			if l.pos >= len(l.src) {
				return Token{}, fmt.Errorf("%s:%d:%d: unterminated string escape", l.filename, line, col)
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

func (l *Lexer) scanNumber(line, col, startPos int) Token {
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

func (l *Lexer) isListMarkerAhead() bool {
	if l.pos >= len(l.src) {
		return true
	}
	r, _ := l.peekN(1)
	return r == ' ' || r == '\t' || r == '\n' || r == '#'
}

func (l *Lexer) peekArrow() bool {
	if l.pos+1 >= len(l.src) {
		return false
	}
	return l.src[l.pos+1] == '>'
}

func (l *Lexer) matchStringPrefix(prefix string) bool {
	return strings.HasPrefix(l.src[l.pos:], prefix)
}

func (l *Lexer) peek() (rune, int) {
	if l.pos >= len(l.src) {
		return utf8.RuneError, 0
	}
	return utf8.DecodeRuneInString(l.src[l.pos:])
}

func (l *Lexer) peekN(n int) (rune, int) {
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

func (l *Lexer) consume() rune {
	if l.pos >= len(l.src) {
		return utf8.RuneError
	}
	r, width := utf8.DecodeRuneInString(l.src[l.pos:])
	l.pos += width
	if r == '\n' {
		l.line++
		l.column = 1
		l.atLineStart = true
		l.lineHasToken = false
		return r
	}
	l.column++
	return r
}

func (l *Lexer) token(kind TokenKind, lexeme string, line, col int) Token {
	return Token{
		Kind:   kind,
		Lexeme: lexeme,
		File:   l.filename,
		Line:   line,
		Column: col,
	}
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r'
}

func isIdentStart(r rune) bool {
	return r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func isIdentPart(r rune) bool {
	return isIdentStart(r) || (r >= '0' && r <= '9')
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isPunctuation(r rune) bool {
	switch r {
	case ':', ',', '.', '|', '<', '>', '(', ')', '[', ']', '{', '}', '=', '?':
		return true
	default:
		return false
	}
}
