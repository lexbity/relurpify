package prompt

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// compiledExpr implements Expression by holding a parsed AST.
type compiledExpr struct {
	root exprNode
	raw  string
}

func (e *compiledExpr) Evaluate(state map[string]any) (bool, error) {
	v, err := evalNode(e.root, state)
	if err != nil {
		return false, fmt.Errorf("expression %q: %w", e.raw, err)
	}
	return truthy(v), nil
}

// compileExpression parses a when-expression string and returns a compiled form.
func compileExpression(s string) (*compiledExpr, error) {
	p := &exprParser{input: strings.TrimSpace(s), pos: 0}
	node, err := p.parseOr()
	if err != nil {
		return nil, fmt.Errorf("parse expression %q: %w", s, err)
	}
	p.skipWS()
	if p.pos != len(p.input) {
		return nil, fmt.Errorf("parse expression %q: unexpected token at position %d", s, p.pos)
	}
	return &compiledExpr{root: node, raw: s}, nil
}

// ---- AST node types --------------------------------------------------------

type exprNode interface{ exprNode() }

type orNode struct{ left, right exprNode }
type andNode struct{ left, right exprNode }
type existsNode struct{ path []string }
type compNode struct {
	left  []string
	op    string
	right exprNode
}
type pathNode struct{ path []string }
type literalNode struct{ val any }

func (orNode) exprNode()      {}
func (andNode) exprNode()     {}
func (existsNode) exprNode()  {}
func (compNode) exprNode()    {}
func (pathNode) exprNode()    {}
func (literalNode) exprNode() {}

// ---- Evaluator -------------------------------------------------------------

func evalNode(n exprNode, state map[string]any) (any, error) {
	switch v := n.(type) {
	case orNode:
		l, err := evalNode(v.left, state)
		if err != nil {
			return false, err
		}
		if truthy(l) {
			return true, nil
		}
		r, err := evalNode(v.right, state)
		if err != nil {
			return false, err
		}
		return truthy(r), nil

	case andNode:
		l, err := evalNode(v.left, state)
		if err != nil {
			return false, err
		}
		if !truthy(l) {
			return false, nil
		}
		r, err := evalNode(v.right, state)
		if err != nil {
			return false, err
		}
		return truthy(r), nil

	case existsNode:
		_, ok := resolvePath(v.path, state)
		return ok, nil

	case pathNode:
		val, _ := resolvePath(v.path, state)
		return val, nil

	case literalNode:
		return v.val, nil

	case compNode:
		lval, lok := resolvePath(v.left, state)
		if !lok {
			lval = nil
		}
		rval, err := evalNode(v.right, state)
		if err != nil {
			return false, err
		}
		return compareValues(lval, v.op, rval)

	default:
		return nil, fmt.Errorf("unknown node type %T", n)
	}
}

func compareValues(left any, op string, right any) (bool, error) {
	switch op {
	case "==":
		return equalValues(left, right), nil
	case "!=":
		return !equalValues(left, right), nil
	case ">", "<", ">=", "<=":
		lf, lok := toFloat(left)
		rf, rok := toFloat(right)
		if !lok || !rok {
			return false, fmt.Errorf("operator %s requires numeric operands", op)
		}
		switch op {
		case ">":
			return lf > rf, nil
		case "<":
			return lf < rf, nil
		case ">=":
			return lf >= rf, nil
		case "<=":
			return lf <= rf, nil
		}
	}
	return false, fmt.Errorf("unknown operator %s", op)
}

func equalValues(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if aok && bok {
		return af == bf
	}
	ab, abok := a.(bool)
	bb, bbok := b.(bool)
	if abok && bbok {
		return ab == bb
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case int32:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	return 0, false
}

func truthy(v any) bool {
	if v == nil {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0
	case float32:
		return x != 0
	case int:
		return x != 0
	case int64:
		return x != 0
	case int32:
		return x != 0
	}
	return true
}

func resolvePath(path []string, state map[string]any) (any, bool) {
	var cur any = state
	for _, seg := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[seg]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// ---- Parser ----------------------------------------------------------------

type exprParser struct {
	input string
	pos   int
}

func (p *exprParser) skipWS() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func (p *exprParser) peek() byte {
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

func (p *exprParser) parseOr() (exprNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		p.skipWS()
		if p.pos+1 < len(p.input) && p.input[p.pos] == '|' && p.input[p.pos+1] == '|' {
			p.pos += 2
			right, err := p.parseAnd()
			if err != nil {
				return nil, err
			}
			left = orNode{left: left, right: right}
		} else {
			break
		}
	}
	return left, nil
}

func (p *exprParser) parseAnd() (exprNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		p.skipWS()
		if p.pos+1 < len(p.input) && p.input[p.pos] == '&' && p.input[p.pos+1] == '&' {
			p.pos += 2
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			left = andNode{left: left, right: right}
		} else {
			break
		}
	}
	return left, nil
}

func (p *exprParser) parseUnary() (exprNode, error) {
	p.skipWS()
	if p.peek() == '(' {
		p.pos++
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		p.skipWS()
		if p.peek() != ')' {
			return nil, fmt.Errorf("expected ')'")
		}
		p.pos++
		return inner, nil
	}
	return p.parsePrimary()
}

func (p *exprParser) parsePrimary() (exprNode, error) {
	p.skipWS()
	if p.peek() == '"' || p.peek() == '\'' {
		return p.parseLiteralString()
	}
	if p.peek() == '-' || (p.peek() >= '0' && p.peek() <= '9') {
		return p.parseLiteralNumber()
	}
	path, err := p.parsePath()
	if err != nil {
		return nil, err
	}
	p.skipWS()
	if p.matchWord("exists") {
		return existsNode{path: path}, nil
	}
	p.skipWS()
	op := p.parseOp()
	if op != "" {
		p.skipWS()
		right, err := p.parseOperand()
		if err != nil {
			return nil, err
		}
		return compNode{left: path, op: op, right: right}, nil
	}
	if len(path) == 1 {
		switch path[0] {
		case "true":
			return literalNode{val: true}, nil
		case "false":
			return literalNode{val: false}, nil
		}
	}
	return pathNode{path: path}, nil
}

func (p *exprParser) parsePath() ([]string, error) {
	p.skipWS()
	var segs []string
	for {
		seg, err := p.parseIdentifier()
		if err != nil {
			if len(segs) == 0 {
				return nil, err
			}
			p.pos-- // back off the trailing dot
			break
		}
		segs = append(segs, seg)
		if p.peek() != '.' {
			break
		}
		p.pos++
	}
	return segs, nil
}

func (p *exprParser) parseIdentifier() (string, error) {
	start := p.pos
	if p.pos >= len(p.input) {
		return "", fmt.Errorf("expected identifier at position %d", p.pos)
	}
	c := rune(p.input[p.pos])
	if !unicode.IsLetter(c) && c != '_' {
		return "", fmt.Errorf("expected identifier at position %d, got %q", p.pos, c)
	}
	p.pos++
	for p.pos < len(p.input) {
		c := rune(p.input[p.pos])
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' {
			break
		}
		p.pos++
	}
	return p.input[start:p.pos], nil
}

func (p *exprParser) parseOp() string {
	ops := []string{"==", "!=", ">=", "<=", ">", "<"}
	for _, op := range ops {
		if strings.HasPrefix(p.input[p.pos:], op) {
			p.pos += len(op)
			return op
		}
	}
	return ""
}

func (p *exprParser) parseOperand() (exprNode, error) {
	p.skipWS()
	if p.peek() == '"' || p.peek() == '\'' {
		return p.parseLiteralString()
	}
	if p.peek() == '-' || (p.peek() >= '0' && p.peek() <= '9') {
		return p.parseLiteralNumber()
	}
	path, err := p.parsePath()
	if err != nil {
		return nil, err
	}
	if len(path) == 1 {
		switch path[0] {
		case "true":
			return literalNode{val: true}, nil
		case "false":
			return literalNode{val: false}, nil
		}
	}
	return pathNode{path: path}, nil
}

func (p *exprParser) parseLiteralString() (exprNode, error) {
	quote := p.input[p.pos]
	p.pos++
	var b strings.Builder
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		p.pos++
		if c == quote {
			return literalNode{val: b.String()}, nil
		}
		if c == '\\' && p.pos < len(p.input) {
			b.WriteByte(p.input[p.pos])
			p.pos++
			continue
		}
		b.WriteByte(c)
	}
	return nil, fmt.Errorf("unterminated string literal")
}

func (p *exprParser) parseLiteralNumber() (exprNode, error) {
	start := p.pos
	if p.peek() == '-' {
		p.pos++
	}
	for p.pos < len(p.input) && (p.input[p.pos] >= '0' && p.input[p.pos] <= '9') {
		p.pos++
	}
	if p.pos < len(p.input) && p.input[p.pos] == '.' {
		p.pos++
		for p.pos < len(p.input) && (p.input[p.pos] >= '0' && p.input[p.pos] <= '9') {
			p.pos++
		}
	}
	f, err := strconv.ParseFloat(p.input[start:p.pos], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid number %q", p.input[start:p.pos])
	}
	return literalNode{val: f}, nil
}

func (p *exprParser) matchWord(word string) bool {
	if !strings.HasPrefix(p.input[p.pos:], word) {
		return false
	}
	after := p.pos + len(word)
	if after < len(p.input) {
		c := rune(p.input[after])
		if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' {
			return false
		}
	}
	p.pos += len(word)
	return true
}
