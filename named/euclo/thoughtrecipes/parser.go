package thoughtrecipe

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Parser converts Euclo tokens into the source-level AST.
type Parser struct {
	filename string
	tokens   []Token
	pos      int
}

// NewParser creates a parser for a token stream.
func NewParser(filename string, tokens []Token) *Parser {
	return &Parser{filename: filename, tokens: tokens}
}

// ParseSource tokenizes and parses a Euclo source file.
func ParseSource(filename, src string) (*ThoughtRecipeDocument, error) {
	tokens, err := NewLexer(filename, src).LexAll()
	if err != nil {
		return nil, err
	}
	return NewParser(filename, tokens).ParseDocument()
}

// ParseDocument parses a full Euclo thoughtrecipe document.
func (p *Parser) ParseDocument() (*ThoughtRecipeDocument, error) {
	header, err := p.parseHeader()
	if err != nil {
		return nil, err
	}

	doc := &ThoughtRecipeDocument{
		SourcePath: p.filename,
		Name:       header.Name.Value,
		Header:     header,
	}

	for !p.atEOF() {
		if p.peek().Kind == TokenDedent {
			return nil, p.unexpectedToken(p.peek(), "unexpected dedent at top level")
		}
		decl, err := p.parseTopLevelDeclaration()
		if err != nil {
			return nil, err
		}
		doc.Declarations = append(doc.Declarations, decl)
	}

	return doc, nil
}

func (p *Parser) parseHeader() (ThoughtRecipeHeader, error) {
	thoughtrecipeTok, err := p.expectKeyword("thoughtrecipe")
	if err != nil {
		return ThoughtRecipeHeader{}, err
	}
	nameTok, err := p.expectName("thoughtrecipe name")
	if err != nil {
		return ThoughtRecipeHeader{}, err
	}

	header := ThoughtRecipeHeader{
		positioned: positioned{Span: spanFromTokens(thoughtrecipeTok, nameTok)},
		Name:       Identifier{positioned: positioned{Span: spanFromToken(nameTok)}, Value: nameTok.Lexeme},
	}

	if !p.atEOF() && p.peek().Kind == TokenString {
		descTok := p.next()
		header.Description = &StringLiteral{
			positioned: positioned{Span: spanFromToken(descTok)},
			Raw:        descTok.Lexeme,
			Value:      unquoteString(descTok.Lexeme),
		}
		header.Span = spanFromTokens(thoughtrecipeTok, descTok)
	}

	return header, nil
}

func (p *Parser) parseTopLevelDeclaration() (Declaration, error) {
	switch p.peek().Lexeme {
	case "import":
		return p.parseImportDecl()
	case "trigger":
		return p.parseTriggerDecl()
	case "input":
		return p.parseInputDecl()
	case "type":
		return p.parseTypeDecl()
	case "agent":
		return p.parseAgentDecl()
	case "run":
		return p.parseRunDecl()
	case "route":
		return p.parseRouteDecl()
	case "delegate":
		return p.parseDelegateDecl()
	case "ask":
		return p.parseAskDecl()
	case "pipeline":
		return p.parsePipelineDecl()
	default:
		return nil, p.unexpectedToken(p.peek(), "unknown top-level declaration")
	}
}

func (p *Parser) parseImportDecl() (*ImportDecl, error) {
	start := p.next()
	kindTok, err := p.expectKeyword("prompt")
	if err != nil {
		if _, recipeErr := p.expectKeyword("recipe"); recipeErr != nil {
			return nil, p.unexpectedToken(p.peek(), "expected prompt or recipe import kind")
		}
		kindTok = Token{Lexeme: "recipe", File: start.File, Line: start.Line, Column: start.Column}
	}
	kind := ImportKind(kindTok.Lexeme)
	if kind != ImportKindPrompt && kind != ImportKindRecipe {
		return nil, p.unexpectedToken(kindTok, "expected prompt or recipe import kind")
	}
	target, err := p.parsePathExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expectKeyword("as"); err != nil {
		return nil, err
	}
	aliasTok, err := p.expectName("import alias")
	if err != nil {
		return nil, err
	}
	return &ImportDecl{
		positioned: positioned{Span: spanFromTokens(start, aliasTok)},
		Kind:       kind,
		Target:     target,
		Alias:      Identifier{positioned: positioned{Span: spanFromToken(aliasTok)}, Value: aliasTok.Lexeme},
	}, nil
}

func (p *Parser) parseTriggerDecl() (*TriggerDecl, error) {
	start := p.next()
	if _, err := p.expectKeyword("as"); err != nil {
		return nil, err
	}
	policyTok, err := p.expectName("trigger policy")
	if err != nil {
		return nil, err
	}
	colon, err := p.expectPunctuation(":")
	if err != nil {
		return nil, err
	}
	if _, err := p.expectKind(TokenIndent, "trigger block"); err != nil {
		return nil, err
	}

	decl := &TriggerDecl{
		positioned: positioned{Span: spanFromTokens(start, colon)},
		Policy:     Identifier{positioned: positioned{Span: spanFromToken(policyTok)}, Value: policyTok.Lexeme},
		RouteKind:  TriggerRouteKind(strings.ToLower(strings.TrimSpace(policyTok.Lexeme))),
	}

	for !p.atEOF() && p.peek().Kind != TokenDedent {
		switch p.peek().Lexeme {
		case "may":
			mayTok, err := p.expectKeyword("may")
			if err != nil {
				return nil, err
			}
			if p.peekLexeme("invoke") {
				policy, err := p.parseToolInvokePolicyDecl(mayTok)
				if err != nil {
					return nil, err
				}
				decl.ToolPolicies = append(decl.ToolPolicies, *policy)
				continue
			}
			effectTok, err := p.expectName("trigger effect")
			if err != nil {
				return nil, err
			}
			resource, err := p.parseValueExpr()
			if err != nil {
				return nil, err
			}
			decl.Lines = append(decl.Lines, TriggerLine{
				positioned: positioned{Span: spanFromTokens(mayTok, endToken(resource))},
				Effect:     Identifier{positioned: positioned{Span: spanFromToken(effectTok)}, Value: effectTok.Lexeme},
				Resource:   resource,
				Raw:        joinTokens(mayTok, effectTok, resource),
			})
		case TriggerAssociationFamily, TriggerAssociationKeyword, TriggerAssociationHandoff:
			assoc, err := p.parseTriggerAssociationDecl()
			if err != nil {
				return nil, err
			}
			decl.Associations = append(decl.Associations, *assoc)
		default:
			return nil, p.unexpectedToken(p.peek(), "expected trigger policy, family, keyword, or handoff line")
		}
	}

	if _, err := p.expectKind(TokenDedent, "end trigger block"); err != nil {
		return nil, err
	}

	return decl, nil
}

func (p *Parser) parseTriggerAssociationDecl() (*TriggerAssociationDecl, error) {
	start := p.next()
	list, err := p.parseInlineList()
	if err != nil {
		return nil, err
	}
	return &TriggerAssociationDecl{
		positioned: positioned{Span: spanFromTokens(start, endToken(list))},
		Name:       Identifier{positioned: positioned{Span: spanFromToken(start)}, Value: start.Lexeme},
		Values:     list,
		Raw:        strings.TrimSpace(start.Lexeme + " " + list.Raw),
	}, nil
}

func (p *Parser) parseInputDecl() (*InputDecl, error) {
	start := p.next()
	nameTok, err := p.expectName("input name")
	if err != nil {
		return nil, err
	}
	if _, err := p.expectPunctuation(":"); err != nil {
		return nil, err
	}
	source, err := p.parseValueExpr()
	if err != nil {
		return nil, err
	}
	return &InputDecl{
		positioned: positioned{Span: spanFromTokens(start, endToken(source))},
		Name:       Identifier{positioned: positioned{Span: spanFromToken(nameTok)}, Value: nameTok.Lexeme},
		Source:     source,
	}, nil
}

func (p *Parser) parseTypeDecl() (*TypeDecl, error) {
	start := p.next()
	nameTok, err := p.expectName("type name")
	if err != nil {
		return nil, err
	}
	if _, err := p.expectPunctuation(":"); err != nil {
		return nil, err
	}
	if _, err := p.expectKind(TokenIndent, "type body"); err != nil {
		return nil, err
	}

	body := &RecordTypeDefinition{}
	body.Span = spanFromTokens(start, start)

	for !p.atEOF() && p.peek().Kind != TokenDedent {
		fieldTok, err := p.expectName("field name")
		if err != nil {
			return nil, err
		}
		if _, err := p.expectPunctuation(":"); err != nil {
			return nil, err
		}
		fieldType, err := p.parseTypeExpr()
		if err != nil {
			return nil, err
		}
		body.Fields = append(body.Fields, TypeField{
			positioned: positioned{Span: spanFromTokens(fieldTok, endToken(fieldType))},
			Name:       Identifier{positioned: positioned{Span: spanFromToken(fieldTok)}, Value: fieldTok.Lexeme},
			Type:       fieldType,
		})
	}

	endTok, err := p.expectKind(TokenDedent, "end type body")
	if err != nil {
		return nil, err
	}
	body.Span = spanFromTokens(start, endTok)

	return &TypeDecl{
		positioned: positioned{Span: body.Span},
		Name:       DefinitionName{positioned: positioned{Span: spanFromToken(nameTok)}, Value: nameTok.Lexeme},
		Body:       body,
	}, nil
}

func (p *Parser) parseAgentDecl() (*AgentDecl, error) {
	start := p.next()
	nameTok, err := p.expectName("agent name")
	if err != nil {
		return nil, err
	}
	if _, err := p.expectKeyword("uses"); err != nil {
		return nil, err
	}
	typeTok, err := p.expectName("agent type")
	if err != nil {
		return nil, err
	}
	return &AgentDecl{
		positioned: positioned{Span: spanFromTokens(start, typeTok)},
		Name:       Identifier{positioned: positioned{Span: spanFromToken(nameTok)}, Value: nameTok.Lexeme},
		AgentType:  Identifier{positioned: positioned{Span: spanFromToken(typeTok)}, Value: typeTok.Lexeme},
	}, nil
}

func (p *Parser) parseRunDecl() (*RunDecl, error) {
	start := p.next()
	agentTok, err := p.expectName("run agent")
	if err != nil {
		return nil, err
	}
	if _, err := p.expectPunctuation(":"); err != nil {
		return nil, err
	}
	if _, err := p.expectKind(TokenIndent, "run body"); err != nil {
		return nil, err
	}
	items, endTok, err := p.parseRunBlockItems()
	if err != nil {
		return nil, err
	}
	return &RunDecl{
		positioned: positioned{Span: spanFromTokens(start, endTok)},
		Agent:      Identifier{positioned: positioned{Span: spanFromToken(agentTok)}, Value: agentTok.Lexeme},
		Items:      items,
	}, nil
}

func (p *Parser) parseDelegateDecl() (*DelegateDecl, error) {
	start := p.next()
	if _, err := p.expectKeyword("to"); err != nil {
		return nil, err
	}
	agentTok, err := p.expectName("delegate target")
	if err != nil {
		return nil, err
	}
	if _, err := p.expectPunctuation(":"); err != nil {
		return nil, err
	}
	if _, err := p.expectKind(TokenIndent, "delegate body"); err != nil {
		return nil, err
	}
	items, endTok, err := p.parseExecutionBlockItems()
	if err != nil {
		return nil, err
	}
	return &DelegateDecl{
		positioned: positioned{Span: spanFromTokens(start, endTok)},
		Agent:      Identifier{positioned: positioned{Span: spanFromToken(agentTok)}, Value: agentTok.Lexeme},
		Items:      items,
	}, nil
}

func (p *Parser) parseRouteDecl() (*RouteDecl, error) {
	start := p.next()
	if _, err := p.expectPunctuation(":"); err != nil {
		return nil, err
	}
	if _, err := p.expectKind(TokenIndent, "route body"); err != nil {
		return nil, err
	}

	var branches []RouteBranch
	for !p.atEOF() && p.peek().Kind != TokenDedent {
		branch, err := p.parseRouteBranch()
		if err != nil {
			return nil, err
		}
		branches = append(branches, branch)
	}
	endTok, err := p.expectKind(TokenDedent, "end route body")
	if err != nil {
		return nil, err
	}
	return &RouteDecl{positioned: positioned{Span: spanFromTokens(start, endTok)}, Branches: branches}, nil
}

func (p *Parser) parseRouteBranch() (RouteBranch, error) {
	start := p.peek()
	branch := RouteBranch{}
	if p.peekLexeme("when") {
		p.next()
		pred, err := p.parsePredicateExpr()
		if err != nil {
			return RouteBranch{}, err
		}
		branch.Predicate = pred
		branch.positioned = positioned{Span: pred.Span}
		if _, err := p.expectPunctuation(":"); err != nil {
			return RouteBranch{}, err
		}
		if _, err := p.expectKind(TokenIndent, "route branch"); err != nil {
			return RouteBranch{}, err
		}
		items, endTok, err := p.parseExecutionBlockItems()
		if err != nil {
			return RouteBranch{}, err
		}
		branch.Body = items
		branch.Span = spanFromTokens(start, endTok)
		return branch, nil
	}
	if p.peekLexeme("otherwise") {
		p.next()
		branch.IsElse = true
		if _, err := p.expectPunctuation(":"); err != nil {
			return RouteBranch{}, err
		}
		if _, err := p.expectKind(TokenIndent, "route branch"); err != nil {
			return RouteBranch{}, err
		}
		items, endTok, err := p.parseExecutionBlockItems()
		if err != nil {
			return RouteBranch{}, err
		}
		branch.Body = items
		branch.Span = spanFromTokens(start, endTok)
		return branch, nil
	}
	return RouteBranch{}, p.unexpectedToken(start, "expected when or otherwise")
}

func (p *Parser) parseAskDecl() (*AskDecl, error) {
	start := p.next()
	if _, err := p.expectKeyword("user"); err != nil {
		return nil, err
	}
	if _, err := p.expectPunctuation(":"); err != nil {
		return nil, err
	}
	if _, err := p.expectKind(TokenIndent, "ask body"); err != nil {
		return nil, err
	}
	var items []AskItem
	for !p.atEOF() && p.peek().Kind != TokenDedent {
		item, err := p.parseAskItem()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	endTok, err := p.expectKind(TokenDedent, "end ask body")
	if err != nil {
		return nil, err
	}
	return &AskDecl{positioned: positioned{Span: spanFromTokens(start, endTok)}, Items: items}, nil
}

func (p *Parser) parsePipelineDecl() (*PipelineDecl, error) {
	start := p.next()
	if _, err := p.expectPunctuation(":"); err != nil {
		return nil, err
	}
	if _, err := p.expectKind(TokenIndent, "pipeline body"); err != nil {
		return nil, err
	}
	var stages []PipelineStage
	for !p.atEOF() && p.peek().Kind != TokenDedent {
		stage, err := p.parsePipelineStage()
		if err != nil {
			return nil, err
		}
		stages = append(stages, stage)
	}
	endTok, err := p.expectKind(TokenDedent, "end pipeline body")
	if err != nil {
		return nil, err
	}
	return &PipelineDecl{positioned: positioned{Span: spanFromTokens(start, endTok)}, Stages: stages}, nil
}

func (p *Parser) parseRunBlockItems() ([]ExecutionItem, Token, error) {
	var items []ExecutionItem
	var last Token
	for !p.atEOF() && p.peek().Kind != TokenDedent {
		item, err := p.parseRunExecutionItem()
		if err != nil {
			return nil, Token{}, err
		}
		items = append(items, item)
		last = endToken(item)
	}
	endTok, err := p.expectKind(TokenDedent, "end block")
	if err != nil {
		return nil, Token{}, err
	}
	if last.Kind == TokenIllegal {
		last = endTok
	}
	return items, endTok, nil
}

func (p *Parser) parsePipelineStage() (PipelineStage, error) {
	start := p.peek()
	if _, err := p.expectKeyword("stage"); err != nil {
		return PipelineStage{}, err
	}
	nameTok, err := p.expectName("stage name")
	if err != nil {
		return PipelineStage{}, err
	}
	if _, err := p.expectPunctuation(":"); err != nil {
		return PipelineStage{}, err
	}
	if _, err := p.expectKind(TokenIndent, "stage body"); err != nil {
		return PipelineStage{}, err
	}
	items, endTok, err := p.parseExecutionBlockItems()
	if err != nil {
		return PipelineStage{}, err
	}
	return PipelineStage{
		positioned: positioned{Span: spanFromTokens(start, endTok)},
		Name:       Identifier{positioned: positioned{Span: spanFromToken(nameTok)}, Value: nameTok.Lexeme},
		Body:       items,
	}, nil
}

func (p *Parser) parseExecutionBlockItems() ([]ExecutionItem, Token, error) {
	var items []ExecutionItem
	var last Token
	for !p.atEOF() && p.peek().Kind != TokenDedent {
		item, err := p.parseExecutionItem()
		if err != nil {
			return nil, Token{}, err
		}
		items = append(items, item)
		last = endToken(item)
	}
	endTok, err := p.expectKind(TokenDedent, "end block")
	if err != nil {
		return nil, Token{}, err
	}
	if last.Kind == TokenIllegal {
		last = endTok
	}
	return items, endTok, nil
}

func (p *Parser) parseRunExecutionItem() (ExecutionItem, error) {
	if p.peekLexeme("may") {
		mayTok := p.peek()
		p.next()
		if !p.peekLexeme("invoke") {
			return nil, p.unexpectedToken(mayTok, "may invoke is the only may policy allowed in run blocks")
		}
		return p.parseToolInvokePolicyDecl(mayTok)
	}
	return p.parseExecutionItem()
}

func (p *Parser) parseExecutionItem() (ExecutionItem, error) {
	switch p.peek().Lexeme {
	case "run":
		return p.parseRunDecl()
	case "delegate":
		return p.parseDelegateDecl()
	case "route":
		return p.parseRouteDecl()
	case "ask":
		return p.parseAskDecl()
	case "pipeline":
		return p.parsePipelineDecl()
	case "from":
		return p.parseFromClause()
	case "goal":
		return p.parseGoalClause()
	case "do":
		return p.parseCapabilityInvocation()
	case "capture":
		return p.parseCaptureBlock()
	case "step", "task", "method", "link", "source", "detect", "clarify", "revise", "retry", "plan", "verify", "summarize", "review", "decompose", "solve", "until":
		return p.parseDirectiveItem()
	default:
		if p.peek().Kind == TokenKeyword || p.peek().Kind == TokenIdentifier {
			return p.parseDirectiveItem()
		}
		return nil, p.unexpectedToken(p.peek(), "unsupported execution item")
	}
}

func (p *Parser) parseToolInvokePolicyDecl(mayTok Token) (*ToolInvokePolicyDecl, error) {
	invokeTok, err := p.expectKeyword("invoke")
	if err != nil {
		return nil, p.unexpectedToken(p.peek(), `expected "invoke" after "may"`)
	}
	list, err := p.parseToolNameList()
	if err != nil {
		return nil, err
	}
	return &ToolInvokePolicyDecl{
		positioned: positioned{Span: spanFromTokens(mayTok, endToken(list))},
		ToolNames:  list,
		Raw:        strings.TrimSpace(joinTokens(mayTok, invokeTok, list)),
	}, nil
}

func (p *Parser) parseFromClause() (*FromClause, error) {
	start := p.next()
	source, err := p.parseValueExpr()
	if err != nil {
		return nil, err
	}
	return &FromClause{positioned: positioned{Span: spanFromTokens(start, endToken(source))}, Source: source}, nil
}

func (p *Parser) parseGoalClause() (*GoalClause, error) {
	start := p.next()
	switch p.peek().Lexeme {
	case "prompt":
		ref, err := p.parsePromptRef()
		if err != nil {
			return nil, err
		}
		return &GoalClause{positioned: positioned{Span: spanFromTokens(start, tokenFromSpan(ref.Span))}, PromptID: ref}, nil
	default:
		str, err := p.expectStringLiteral("goal text")
		if err != nil {
			return nil, err
		}
		return &GoalClause{positioned: positioned{Span: spanFromTokens(start, tokenFromSpan(str.Span))}, Text: *str}, nil
	}
}

func (p *Parser) parseCapabilityInvocation() (*CapabilityInvocation, error) {
	start := p.next()
	nsTok, err := p.expectName("capability namespace")
	if err != nil {
		return nil, err
	}
	if _, err := p.expectPunctuation(":"); err != nil {
		return nil, err
	}
	capTok, err := p.expectName("capability name")
	if err != nil {
		return nil, err
	}
	item := &CapabilityInvocation{
		positioned: positioned{Span: spanFromTokens(start, capTok)},
		Namespace:  Identifier{positioned: positioned{Span: spanFromToken(nsTok)}, Value: nsTok.Lexeme},
		Capability: Identifier{positioned: positioned{Span: spanFromToken(capTok)}, Value: capTok.Lexeme},
	}
	if p.peekLexeme("on") {
		p.next()
		target, err := p.parseValueExpr()
		if err != nil {
			return nil, err
		}
		item.Target = target
		item.Span = spanFromTokens(start, endToken(target))
	}
	if p.peekLexeme("with") {
		p.next()
		input, err := p.parseValueExpr()
		if err != nil {
			return nil, err
		}
		item.Input = input
		item.Span = spanFromTokens(start, endToken(input))
	}
	return item, nil
}

func (p *Parser) parseCaptureBlock() (*CaptureBlock, error) {
	start := p.next()
	block := &CaptureBlock{positioned: positioned{Span: spanFromToken(start)}}
	if p.peekKind(TokenPunctuation, ":") {
		p.next()
		if _, err := p.expectKind(TokenIndent, "capture block"); err != nil {
			return nil, err
		}
		for !p.atEOF() && p.peek().Kind != TokenDedent {
			binding, err := p.parseCaptureBinding()
			if err != nil {
				return nil, err
			}
			block.Bindings = append(block.Bindings, binding)
		}
		endTok, err := p.expectKind(TokenDedent, "end capture block")
		if err != nil {
			return nil, err
		}
		block.Span = spanFromTokens(start, endTok)
		return block, nil
	}

	binding, err := p.parseCaptureBinding()
	if err != nil {
		return nil, err
	}
	block.Bindings = []CaptureBinding{binding}
	block.Inline = true
	block.Span = spanFromTokens(start, endToken(binding))
	return block, nil
}

func (p *Parser) parseCaptureBinding() (CaptureBinding, error) {
	start := p.peek()
	source, err := p.parseValueExpr()
	if err != nil {
		return CaptureBinding{}, err
	}
	binding := CaptureBinding{
		positioned: positioned{Span: spanFromToken(start)},
		Source:     source,
	}
	if p.peekKind(TokenPunctuation, ":") {
		p.next()
		annotation, err := p.parseTypeExpr()
		if err != nil {
			return CaptureBinding{}, err
		}
		binding.Annotation = annotation
	}
	if _, err := p.expectArrow(); err != nil {
		return CaptureBinding{}, err
	}
	dest, err := p.parsePathExpr()
	if err != nil {
		return CaptureBinding{}, err
	}
	binding.Destination = dest
	if binding.Annotation == nil {
		switch source.(type) {
		case PathExpr, *PathExpr:
			binding.Forwarding = true
		}
	}
	binding.Span = spanFromTokens(start, endToken(dest))
	return binding, nil
}

func (p *Parser) parseDirectiveItem() (ExecutionItem, error) {
	start := p.peek()
	nameTok := p.next()
	rawTokens := []Token{nameTok}
	var args []ValueExpr
	var pred *PredicateExpr

	if nameTok.Lexeme == "may" {
		if p.peekLexeme("invoke") {
			return nil, p.unexpectedToken(p.peek(), "may invoke is only allowed in trigger and run blocks")
		}
		return nil, p.unexpectedToken(nameTok, "may policy lines are only allowed in trigger blocks")
	}

	if nameTok.Lexeme == "when" {
		pexpr, err := p.parsePredicateExpr()
		if err != nil {
			return nil, err
		}
		rawTokens = append(rawTokens, endToken(pexpr))
		return &DirectiveClause{
			positioned: positioned{Span: spanFromTokens(start, endToken(pexpr))},
			Name:       Identifier{positioned: positioned{Span: spanFromToken(nameTok)}, Value: nameTok.Lexeme},
			Raw:        collectRaw(rawTokens),
		}, nil
	}
	if nameTok.Lexeme == "otherwise" {
		return &DirectiveClause{
			positioned: positioned{Span: spanFromToken(nameTok)},
			Name:       Identifier{positioned: positioned{Span: spanFromToken(nameTok)}, Value: nameTok.Lexeme},
			Raw:        nameTok.Lexeme,
		}, nil
	}

	if p.peekLexeme("when") {
		rawTokens = append(rawTokens, p.next())
		pexpr, err := p.parsePredicateExpr()
		if err != nil {
			return nil, err
		}
		pred = &pexpr
		rawTokens = append(rawTokens, endToken(pexpr))
	}

	for !p.atEOF() {
		if p.peek().Kind == TokenPunctuation && p.peek().Lexeme == ":" {
			p.next()
			if _, err := p.expectKind(TokenIndent, "directive block"); err != nil {
				return nil, err
			}
			body, endTok, err := p.parseExecutionBlockItems()
			if err != nil {
				return nil, err
			}
			return &DirectiveBlock{
				positioned: positioned{Span: spanFromTokens(start, endTok)},
				Name:       Identifier{positioned: positioned{Span: spanFromToken(nameTok)}, Value: nameTok.Lexeme},
				Arguments:  args,
				Predicate:  pred,
				Raw:        collectRaw(rawTokens),
				Body:       body,
			}, nil
		}
		if p.peek().Line != start.Line {
			break
		}
		if p.peekKind(TokenDedent, "") || p.peekKind(TokenEOF, "") {
			break
		}
		if v, ok, err := p.parseValueExprOnLine(start.Line); err != nil {
			return nil, err
		} else if ok {
			args = append(args, v)
			rawTokens = append(rawTokens, endToken(v))
			continue
		}
		break
	}

	return &DirectiveClause{
		positioned: positioned{Span: spanFromTokens(start, nameTok)},
		Name:       Identifier{positioned: positioned{Span: spanFromToken(nameTok)}, Value: nameTok.Lexeme},
		Arguments:  args,
		Raw:        collectRaw(rawTokens),
	}, nil
}

func (p *Parser) parseAskItem() (AskItem, error) {
	switch p.peek().Lexeme {
	case "question":
		start := p.next()
		switch p.peek().Lexeme {
		case "prompt":
			ref, err := p.parsePromptRef()
			if err != nil {
				return nil, err
			}
			return &QuestionClause{positioned: positioned{Span: spanFromTokens(start, tokenFromSpan(ref.Span))}, PromptID: ref}, nil
		default:
			text, err := p.expectStringLiteral("question text")
			if err != nil {
				return nil, err
			}
			return &QuestionClause{positioned: positioned{Span: spanFromTokens(start, tokenFromSpan(text.Span))}, Text: *text}, nil
		}
	case "choices":
		start := p.next()
		if p.peekLexeme("from") {
			p.next()
			source, err := p.parseValueExpr()
			if err != nil {
				return nil, err
			}
			return &ChoicesReferenceClause{positioned: positioned{Span: spanFromTokens(start, endToken(source))}, Source: source}, nil
		}
		if p.peekKind(TokenPunctuation, "[") {
			list, err := p.parseInlineList()
			if err != nil {
				return nil, err
			}
			return &ChoicesListClause{positioned: positioned{Span: spanFromTokens(start, endToken(list))}, Raw: list.Raw, Items: list.Entries}, nil
		}
		if p.peekKind(TokenPunctuation, ":") {
			p.next()
			if _, err := p.expectKind(TokenIndent, "choices block"); err != nil {
				return nil, err
			}
			list, err := p.parseBlockList()
			if err != nil {
				return nil, err
			}
			endTok, err := p.expectKind(TokenDedent, "end choices block")
			if err != nil {
				return nil, err
			}
			return &ChoicesListClause{positioned: positioned{Span: spanFromTokens(start, endTok)}, Raw: list.Raw, Items: list.Entries}, nil
		}
		return nil, p.unexpectedToken(p.peek(), "expected choices source or list")
	case "capture":
		capture, err := p.parseCaptureBlock()
		if err != nil {
			return nil, err
		}
		return capture, nil
	default:
		return nil, p.unexpectedToken(p.peek(), "unsupported ask item")
	}
}

func (p *Parser) parsePromptRef() (*PromptRef, error) {
	start := p.peek()
	if _, err := p.expectKeyword("prompt"); err != nil {
		return nil, err
	}
	nameTok, err := p.expectName("prompt binding")
	if err != nil {
		return nil, err
	}
	return &PromptRef{
		positioned: positioned{Span: spanFromTokens(start, nameTok)},
		Name:       Identifier{positioned: positioned{Span: spanFromToken(nameTok)}, Value: nameTok.Lexeme},
	}, nil
}

func (p *Parser) parsePredicateExpr() (PredicateExpr, error) {
	start := p.peek()
	rawTokens := []Token{}
	switch p.peek().Lexeme {
	case "missing":
		p.next()
		subject, err := p.parsePathExpr()
		if err != nil {
			return PredicateExpr{}, err
		}
		rawTokens = append(rawTokens, start, endToken(subject))
		return PredicateExpr{
			positioned: positioned{Span: spanFromTokens(start, endToken(subject))},
			Raw:        collectRaw(rawTokens),
			Kind:       "missing",
			Subject:    subject,
		}, nil
	case "present":
		p.next()
		subject, err := p.parsePathExpr()
		if err != nil {
			return PredicateExpr{}, err
		}
		rawTokens = append(rawTokens, start, endToken(subject))
		return PredicateExpr{
			positioned: positioned{Span: spanFromTokens(start, endToken(subject))},
			Raw:        collectRaw(rawTokens),
			Kind:       "present",
			Subject:    subject,
		}, nil
	default:
		subject, err := p.parsePathExpr()
		if err != nil {
			return PredicateExpr{}, err
		}
		opTok, err := p.expectName("predicate operator")
		if err != nil {
			return PredicateExpr{}, err
		}
		rawTokens = append(rawTokens, start, endToken(subject), opTok)
		pred := PredicateExpr{
			positioned: positioned{Span: spanFromTokens(start, opTok)},
			Subject:    subject,
			Operator:   opTok.Lexeme,
		}
		switch opTok.Lexeme {
		case "confidence":
			if _, err := p.expectKeyword("below"); err != nil {
				return PredicateExpr{}, err
			}
			num, err := p.expectNumber("confidence threshold")
			if err != nil {
				return PredicateExpr{}, err
			}
			pred.Kind = "confidence_below"
			pred.Value = &NumberLiteral{positioned: positioned{Span: spanFromToken(num)}, Raw: num.Lexeme, Value: num.Lexeme}
			pred.Span = spanFromTokens(start, num)
			rawTokens = append(rawTokens, num)
			pred.Raw = collectRaw(rawTokens)
			return pred, nil
		case "is", "contains":
			val, err := p.parseValueExpr()
			if err != nil {
				return PredicateExpr{}, err
			}
			pred.Kind = opTok.Lexeme
			pred.Value = val
			pred.Span = spanFromTokens(start, endToken(val))
			rawTokens = append(rawTokens, endToken(val))
			pred.Raw = collectRaw(rawTokens)
			return pred, nil
		default:
			return PredicateExpr{}, p.unexpectedToken(opTok, "unsupported predicate operator")
		}
	}
}

func (p *Parser) parseValueExpr() (ValueExpr, error) {
	v, _, err := p.parseValueExprOnLine(p.peek().Line)
	return v, err
}

func (p *Parser) parseValueExprOnLine(line int) (ValueExpr, bool, error) {
	if p.atEOF() {
		return nil, false, nil
	}
	tok := p.peek()
	if tok.Line != line {
		return nil, false, nil
	}
	switch tok.Kind {
	case TokenString:
		p.next()
		return &StringLiteral{positioned: positioned{Span: spanFromToken(tok)}, Raw: tok.Lexeme, Value: unquoteString(tok.Lexeme)}, true, nil
	case TokenNumber:
		p.next()
		return &NumberLiteral{positioned: positioned{Span: spanFromToken(tok)}, Raw: tok.Lexeme, Value: tok.Lexeme}, true, nil
	case TokenIdentifier, TokenKeyword:
		path, err := p.parsePathExpr()
		if err != nil {
			return nil, false, err
		}
		return path, true, nil
	case TokenPunctuation:
		if tok.Lexeme == "[" {
			list, err := p.parseInlineList()
			if err != nil {
				return nil, false, err
			}
			return list, true, nil
		}
	}
	return nil, false, nil
}

func (p *Parser) parseInlineList() (*ListLiteral, error) {
	start, err := p.expectPunctuation("[")
	if err != nil {
		return nil, err
	}
	list := &ListLiteral{positioned: positioned{Span: spanFromToken(start)}, Raw: start.Lexeme}
	for !p.atEOF() {
		if p.peekKind(TokenPunctuation, "]") {
			end := p.next()
			list.Span = spanFromTokens(start, end)
			list.Raw = formatListRaw(list.Entries)
			return list, nil
		}
		if p.peekKind(TokenPunctuation, ",") {
			p.next()
			continue
		}
		value, err := p.parseValueExpr()
		if err != nil {
			return nil, err
		}
		list.Entries = append(list.Entries, value)
		if len(list.Entries) == 1 {
			list.Span = spanFromTokens(start, endToken(value))
		} else {
			list.Span = spanFromTokens(start, endToken(value))
		}
	}
	return nil, p.unexpectedEOF("unterminated list literal")
}

func (p *Parser) parseToolNameList() (*ListLiteral, error) {
	start, err := p.expectPunctuation("[")
	if err != nil {
		return nil, p.unexpectedToken(p.peek(), "expected tool name list")
	}
	list := &ListLiteral{positioned: positioned{Span: spanFromToken(start)}, Raw: start.Lexeme}
	for !p.atEOF() {
		if p.peekKind(TokenPunctuation, "]") {
			end := p.next()
			list.Span = spanFromTokens(start, end)
			list.Raw = formatListRaw(list.Entries)
			return list, nil
		}
		if p.peekKind(TokenPunctuation, ",") {
			p.next()
			continue
		}
		tok := p.peek()
		if tok.Kind != TokenString {
			return nil, p.unexpectedToken(tok, "tool name list must contain string literals")
		}
		str, err := p.expectStringLiteral("tool name")
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(str.Value) == "" {
			return nil, p.unexpectedToken(tokenFromSpan(str.Span), "empty tool name is not allowed")
		}
		list.Entries = append(list.Entries, *str)
		list.Span = spanFromTokens(start, tokenFromSpan(str.Span))
	}
	return nil, p.unexpectedEOF("unterminated tool name list")
}

func (p *Parser) parseBlockList() (*ListLiteral, error) {
	list := &ListLiteral{}
	for !p.atEOF() && p.peek().Kind != TokenDedent {
		if !p.peekKind(TokenListMarker, "-") {
			return nil, p.unexpectedToken(p.peek(), "expected list marker")
		}
		start := p.next()
		value, err := p.parseValueExpr()
		if err != nil {
			return nil, err
		}
		list.Entries = append(list.Entries, value)
		if list.Span.Start.Line == 0 {
			list.Span = spanFromTokens(start, endToken(value))
		} else {
			list.Span = spanFromTokens(start, endToken(value))
		}
	}
	list.Raw = formatListRaw(list.Entries)
	return list, nil
}

func (p *Parser) parsePathExpr() (PathExpr, error) {
	start := p.peek()
	parts := make([]Identifier, 0, 2)
	first, err := p.expectName("path segment")
	if err != nil {
		return PathExpr{}, err
	}
	parts = append(parts, Identifier{positioned: positioned{Span: spanFromToken(first)}, Value: first.Lexeme})
	for p.peekKind(TokenPunctuation, ".") {
		p.next()
		nextTok, err := p.expectName("path segment")
		if err != nil {
			return PathExpr{}, err
		}
		parts = append(parts, Identifier{positioned: positioned{Span: spanFromToken(nextTok)}, Value: nextTok.Lexeme})
	}
	return PathExpr{
		positioned: positioned{Span: spanFromTokens(start, p.prevToken())},
		Raw:        joinPath(parts),
		Parts:      parts,
	}, nil
}

func (p *Parser) parseTypeExpr() (TypeExpr, error) {
	type typeFrameKind int

	const (
		typeFrameRoot typeFrameKind = iota
		typeFrameList
		typeFrameOptional
		typeFrameMap
	)

	type typeFrame struct {
		kind  typeFrameKind
		start Token
		ops   []TypeExpr
		key   TypeExpr
		phase int
	}

	collapseOps := func(ops []TypeExpr) (TypeExpr, error) {
		if len(ops) == 0 {
			return nil, p.unexpectedToken(p.peek(), "expected type expression")
		}
		if len(ops) == 1 {
			return ops[0], nil
		}
		start := tokenFromSpan(ops[0].GetSpan())
		end := tokenFromSpan(ops[len(ops)-1].GetSpan())
		return &UnionTypeExpr{
			positioned: positioned{Span: spanFromTokens(start, end)},
			Options:    append([]TypeExpr(nil), ops...),
		}, nil
	}

	finalizeFrame := func(frame *typeFrame, expr TypeExpr) (TypeExpr, error) {
		switch frame.kind {
		case typeFrameList:
			return &ListTypeExpr{
				positioned: positioned{Span: spanFromTokens(frame.start, tokenFromSpan(expr.GetSpan()))},
				Element:    expr,
			}, nil
		case typeFrameOptional:
			return &OptionalTypeExpr{
				positioned: positioned{Span: spanFromTokens(frame.start, tokenFromSpan(expr.GetSpan()))},
				Element:    expr,
			}, nil
		case typeFrameMap:
			if frame.key == nil {
				return nil, p.unexpectedToken(p.peek(), "expected map key expression")
			}
			return &MapTypeExpr{
				positioned: positioned{Span: spanFromTokens(frame.start, tokenFromSpan(expr.GetSpan()))},
				Key:        frame.key,
				Value:      expr,
			}, nil
		default:
			return expr, nil
		}
	}

	frames := []typeFrame{{kind: typeFrameRoot}}
	var result TypeExpr

	for {
		if result == nil {
			tok := p.peek()
			switch tok.Lexeme {
			case "list", "map", "optional":
				start := p.next()
				if _, err := p.expectPunctuation("<"); err != nil {
					return nil, err
				}
				switch start.Lexeme {
				case "list":
					frames = append(frames, typeFrame{kind: typeFrameList, start: start})
				case "optional":
					frames = append(frames, typeFrame{kind: typeFrameOptional, start: start})
				case "map":
					frames = append(frames, typeFrame{kind: typeFrameMap, start: start, phase: 0})
				}
				continue
			default:
				path, err := p.parsePathExpr()
				if err != nil {
					return nil, err
				}
				result = &NamedTypeExpr{positioned: positioned{Span: path.Span}, Name: path}
				continue
			}
		}

		frame := &frames[len(frames)-1]
		switch frame.kind {
		case typeFrameRoot:
			frame.ops = append(frame.ops, result)
			result = nil
			if p.peekKind(TokenPunctuation, "|") {
				p.next()
				continue
			}
			return collapseOps(frame.ops)
		case typeFrameList, typeFrameOptional:
			frame.ops = append(frame.ops, result)
			result = nil
			if p.peekKind(TokenPunctuation, "|") {
				p.next()
				continue
			}
			if !p.peekKind(TokenPunctuation, ">") {
				return nil, p.unexpectedToken(p.peek(), "expected \">\"")
			}
			p.next()
			expr, err := collapseOps(frame.ops)
			if err != nil {
				return nil, err
			}
			wrapped, err := finalizeFrame(frame, expr)
			if err != nil {
				return nil, err
			}
			frames = frames[:len(frames)-1]
			result = wrapped
		case typeFrameMap:
			switch frame.phase {
			case 0:
				frame.ops = append(frame.ops, result)
				result = nil
				if p.peekKind(TokenPunctuation, "|") {
					p.next()
					continue
				}
				if !p.peekKind(TokenPunctuation, ",") {
					return nil, p.unexpectedToken(p.peek(), "expected \",\"")
				}
				key, err := collapseOps(frame.ops)
				if err != nil {
					return nil, err
				}
				frame.key = key
				frame.ops = nil
				frame.phase = 1
				p.next()
			case 1:
				frame.phase = 2
				continue
			case 2:
				frame.ops = append(frame.ops, result)
				result = nil
				if p.peekKind(TokenPunctuation, "|") {
					p.next()
					continue
				}
				if !p.peekKind(TokenPunctuation, ">") {
					return nil, p.unexpectedToken(p.peek(), "expected \">\"")
				}
				p.next()
				value, err := collapseOps(frame.ops)
				if err != nil {
					return nil, err
				}
				mapped, err := finalizeFrame(frame, value)
				if err != nil {
					return nil, err
				}
				frames = frames[:len(frames)-1]
				result = mapped
			default:
				return nil, p.unexpectedToken(p.peek(), "invalid map type state")
			}
		}

		if len(frames) == 0 {
			return nil, p.unexpectedToken(p.peek(), "type parser exhausted frames")
		}
	}
}

// Helper accessors.
func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Kind: TokenEOF, File: p.filename}
	}
	return p.tokens[p.pos]
}

func (p *Parser) next() Token {
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

func (p *Parser) prevToken() Token {
	if p.pos == 0 {
		return Token{Kind: TokenIllegal}
	}
	return p.tokens[p.pos-1]
}

func (p *Parser) atEOF() bool {
	return p.peek().Kind == TokenEOF
}

func (p *Parser) peekKind(kind TokenKind, lexeme string) bool {
	tok := p.peek()
	if tok.Kind != kind {
		return false
	}
	if lexeme != "" && tok.Lexeme != lexeme {
		return false
	}
	return true
}

func (p *Parser) peekLexeme(lexeme string) bool {
	return p.peek().Lexeme == lexeme
}

func (p *Parser) expectKeyword(lexeme string) (Token, error) {
	tok := p.peek()
	if tok.Lexeme != lexeme {
		return Token{}, p.unexpectedToken(tok, fmt.Sprintf("expected %q", lexeme))
	}
	return p.next(), nil
}

func (p *Parser) expectName(context string) (Token, error) {
	tok := p.peek()
	switch tok.Kind {
	case TokenIdentifier, TokenKeyword, TokenNumber:
		return p.next(), nil
	default:
		return Token{}, p.unexpectedToken(tok, "expected "+context)
	}
}

func (p *Parser) expectStringLiteral(context string) (*StringLiteral, error) {
	tok := p.peek()
	if tok.Kind != TokenString {
		return nil, p.unexpectedToken(tok, "expected "+context)
	}
	p.next()
	return &StringLiteral{positioned: positioned{Span: spanFromToken(tok)}, Raw: tok.Lexeme, Value: unquoteString(tok.Lexeme)}, nil
}

func (p *Parser) expectNumber(context string) (Token, error) {
	tok := p.peek()
	if tok.Kind != TokenNumber {
		return Token{}, p.unexpectedToken(tok, "expected "+context)
	}
	return p.next(), nil
}

func (p *Parser) expectPunctuation(lexeme string) (Token, error) {
	tok := p.peek()
	if tok.Kind != TokenPunctuation || tok.Lexeme != lexeme {
		return Token{}, p.unexpectedToken(tok, fmt.Sprintf("expected %q", lexeme))
	}
	return p.next(), nil
}

func (p *Parser) expectArrow() (Token, error) {
	tok := p.peek()
	if tok.Kind == TokenPunctuation && tok.Lexeme == "->" {
		return p.next(), nil
	}
	return Token{}, p.unexpectedToken(tok, "expected \"->\"")
}

func (p *Parser) expectKind(kind TokenKind, context string) (Token, error) {
	tok := p.peek()
	if tok.Kind != kind {
		return Token{}, p.unexpectedToken(tok, "expected "+context)
	}
	return p.next(), nil
}

func (p *Parser) unexpectedToken(tok Token, msg string) error {
	return fmt.Errorf("%s:%d:%d: %s", tok.File, tok.Line, tok.Column, msg)
}

func (p *Parser) unexpectedEOF(msg string) error {
	return fmt.Errorf("%s: unexpected EOF: %s", p.filename, msg)
}

func spanFromToken(tok Token) SourceSpan {
	endCol := tok.Column + utf8.RuneCountInString(tok.Lexeme)
	if endCol > tok.Column {
		endCol--
	}
	return NewSpan(tok.File, tok.Line, tok.Column, tok.Line, endCol)
}

func spanFromTokens(start, end Token) SourceSpan {
	return NewSpan(start.File, start.Line, start.Column, end.Line, end.Column+max(0, utf8.RuneCountInString(end.Lexeme)-1))
}

func endToken(v any) Token {
	switch t := v.(type) {
	case Token:
		return t
	case ValueExpr:
		return endTokenValueExpr(t)
	case TypeExpr:
		return endTokenTypeExpr(t)
	case ExecutionItem:
		return endTokenExecutionItem(t)
	case AskItem:
		return endTokenAskItem(t)
	case PredicateExpr:
		return tokenFromSpan(t.Span)
	case CaptureBinding:
		return endTokenValueExpr(t.Destination)
	default:
		return Token{Kind: TokenIllegal}
	}
}

func endTokenValueExpr(v ValueExpr) Token {
	switch x := v.(type) {
	case PathExpr:
		return tokenFromSpan(x.Span)
	case Identifier:
		return tokenFromSpan(x.Span)
	case StringLiteral:
		return tokenFromSpan(x.Span)
	case NumberLiteral:
		return tokenFromSpan(x.Span)
	case ListLiteral:
		return tokenFromSpan(x.Span)
	case *StringLiteral:
		return tokenFromSpan(x.Span)
	case *NumberLiteral:
		return tokenFromSpan(x.Span)
	case *Identifier:
		return tokenFromSpan(x.Span)
	case *PathExpr:
		return tokenFromSpan(x.Span)
	case *ListLiteral:
		return tokenFromSpan(x.Span)
	default:
		return Token{Kind: TokenIllegal}
	}
}

func endTokenTypeExpr(v TypeExpr) Token {
	switch x := v.(type) {
	case *NamedTypeExpr:
		return tokenFromSpan(x.Span)
	case *ListTypeExpr:
		return tokenFromSpan(x.Span)
	case *OptionalTypeExpr:
		return tokenFromSpan(x.Span)
	case *MapTypeExpr:
		return tokenFromSpan(x.Span)
	case *UnionTypeExpr:
		return tokenFromSpan(x.Span)
	default:
		return Token{Kind: TokenIllegal}
	}
}

func endTokenExecutionItem(v ExecutionItem) Token {
	switch x := v.(type) {
	case *FromClause:
		return tokenFromSpan(x.Span)
	case *GoalClause:
		return tokenFromSpan(x.Span)
	case *DirectiveClause:
		return tokenFromSpan(x.Span)
	case *DirectiveBlock:
		return tokenFromSpan(x.Span)
	case *CapabilityInvocation:
		return tokenFromSpan(x.Span)
	case *ToolInvokePolicyDecl:
		return tokenFromSpan(x.Span)
	case *CaptureBlock:
		return tokenFromSpan(x.Span)
	case *RunDecl:
		return tokenFromSpan(x.Span)
	case *DelegateDecl:
		return tokenFromSpan(x.Span)
	case *RouteDecl:
		return tokenFromSpan(x.Span)
	case *AskDecl:
		return tokenFromSpan(x.Span)
	case *PipelineDecl:
		return tokenFromSpan(x.Span)
	default:
		return Token{Kind: TokenIllegal}
	}
}

func endTokenAskItem(v AskItem) Token {
	switch x := v.(type) {
	case *QuestionClause:
		return tokenFromSpan(x.Span)
	case *ChoicesListClause:
		return tokenFromSpan(x.Span)
	case *ChoicesReferenceClause:
		return tokenFromSpan(x.Span)
	case *CaptureBlock:
		return tokenFromSpan(x.Span)
	default:
		return Token{Kind: TokenIllegal}
	}
}

func tokenFromSpan(span SourceSpan) Token {
	return Token{File: span.Start.File, Line: span.End.Line, Column: span.End.Column}
}

func collectRaw(tokens []Token) string {
	parts := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		if tok.Lexeme != "" {
			parts = append(parts, tok.Lexeme)
		}
	}
	return strings.Join(parts, " ")
}

func formatListRaw(values []ValueExpr) string {
	if len(values) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if raw := strings.TrimSpace(valueExprRaw(value)); raw != "" {
			parts = append(parts, raw)
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func joinTokens(tokens ...any) string {
	parts := make([]string, 0, len(tokens))
	for _, v := range tokens {
		switch x := v.(type) {
		case Token:
			if x.Lexeme != "" {
				parts = append(parts, x.Lexeme)
			}
		case ValueExpr:
			parts = append(parts, valueExprRaw(x))
		case PredicateExpr:
			parts = append(parts, x.Raw)
		}
	}
	return strings.Join(parts, " ")
}

func valueExprRaw(v ValueExpr) string {
	switch x := v.(type) {
	case PathExpr:
		return x.Raw
	case Identifier:
		return x.Value
	case StringLiteral:
		return x.Raw
	case NumberLiteral:
		return x.Raw
	case ListLiteral:
		return x.Raw
	case *StringLiteral:
		return x.Raw
	case *NumberLiteral:
		return x.Raw
	case *Identifier:
		return x.Value
	case *PathExpr:
		return x.Raw
	case *ListLiteral:
		return x.Raw
	default:
		return ""
	}
}

func joinPath(parts []Identifier) string {
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		names = append(names, part.Value)
	}
	return strings.Join(names, ".")
}

func unquoteString(raw string) string {
	if len(raw) >= 6 && strings.HasPrefix(raw, `"""`) && strings.HasSuffix(raw, `"""`) {
		return raw[3 : len(raw)-3]
	}
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		return raw[1 : len(raw)-1]
	}
	return raw
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
