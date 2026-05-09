package thoughtrecipe

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// CapabilityRegistryLookup is the minimal capability lookup contract required
// by semantic validation.
type CapabilityRegistryLookup interface {
	Select(capabilityID string) (core.CapabilityDescriptor, bool)
}

// SemanticWarning captures a non-fatal semantic diagnostic.
type SemanticWarning struct {
	Span    SourceSpan
	Message string
}

// SymbolTable stores the resolved names for a Euclo document.
type SymbolTable struct {
	Document    *ThoughtRecipeDocument
	agents      map[string]*AgentDecl
	inputs      map[string]*InputDecl
	types       map[string]*TypeDecl
	declared    map[string]SourceSpan
	trigger     *TriggerDecl
	invocations []*CapabilityInvocation
	capability  CapabilityRegistryLookup
	Warnings    []SemanticWarning
}

// NewSymbolTable creates a symbol table for the provided document.
func NewSymbolTable(doc *ThoughtRecipeDocument) *SymbolTable {
	return &SymbolTable{
		Document: doc,
		agents:   make(map[string]*AgentDecl),
		inputs:   make(map[string]*InputDecl),
		types:    make(map[string]*TypeDecl),
		declared: make(map[string]SourceSpan),
	}
}

// WithCapabilityRegistry wires capability lookup into the symbol table.
func (s *SymbolTable) WithCapabilityRegistry(reg CapabilityRegistryLookup) *SymbolTable {
	s.capability = reg
	return s
}

// Resolve validates names, namespaces, and references in the document.
func (s *SymbolTable) Resolve() error {
	if s == nil {
		return fmt.Errorf("symbol table is nil")
	}
	if s.Document == nil {
		return fmt.Errorf("thoughtrecipe document is nil")
	}
	if strings.TrimSpace(s.Document.Name) == "" {
		return fmt.Errorf("thoughtrecipe name is required")
	}

	if err := s.collectTopLevelSymbols(); err != nil {
		return err
	}
	if err := s.resolveDeclarations(); err != nil {
		return err
	}
	return s.validateCapabilityPolicy()
}

func (s *SymbolTable) collectTopLevelSymbols() error {
	for _, decl := range s.Document.Declarations {
		switch node := decl.(type) {
		case *InputDecl:
			name := strings.TrimSpace(node.Name.Value)
			if err := s.registerTopLevel(name, node.GetSpan()); err != nil {
				return err
			}
			s.inputs[name] = node
		case *TypeDecl:
			name := strings.TrimSpace(node.Name.Value)
			if err := s.registerTopLevel(name, node.GetSpan()); err != nil {
				return err
			}
			s.types[name] = node
		case *AgentDecl:
			name := strings.TrimSpace(node.Name.Value)
			if err := s.registerTopLevel(name, node.GetSpan()); err != nil {
				return err
			}
			s.agents[name] = node
		}
	}
	return nil
}

func (s *SymbolTable) registerTopLevel(name string, span SourceSpan) error {
	if name == "" {
		return fmt.Errorf("%s:%d:%d: declaration name required", span.Start.File, span.Start.Line, span.Start.Column)
	}
	if prev, exists := s.declared[name]; exists {
		return fmt.Errorf("%s:%d:%d: duplicate declaration %q (previously declared at %s:%d:%d)",
			span.Start.File, span.Start.Line, span.Start.Column, name, prev.Start.File, prev.Start.Line, prev.Start.Column)
	}
	s.declared[name] = span
	return nil
}

func (s *SymbolTable) resolveDeclarations() error {
	for _, decl := range s.Document.Declarations {
		switch node := decl.(type) {
		case *TriggerDecl:
			if err := s.resolveTriggerDecl(node); err != nil {
				return err
			}
		case *InputDecl:
			if err := s.resolveValueExpr(node.Source); err != nil {
				return err
			}
		case *TypeDecl:
			if err := s.resolveTypeDecl(node); err != nil {
				return err
			}
		case *AgentDecl:
			if err := validateAgentParadigm(node); err != nil {
				return err
			}
		case *RunDecl:
			if err := s.resolveRunLike(node.Agent.Value, node.Items, node.GetSpan()); err != nil {
				return err
			}
		case *DelegateDecl:
			if err := s.resolveRunLike(node.Agent.Value, node.Items, node.GetSpan()); err != nil {
				return err
			}
		case *RouteDecl:
			if err := s.resolveRouteDecl(node); err != nil {
				return err
			}
		case *AskDecl:
			if err := s.resolveAskItems(node.Items); err != nil {
				return err
			}
		case *PipelineDecl:
			if err := s.resolvePipelineDecl(node); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *SymbolTable) resolveTriggerDecl(decl *TriggerDecl) error {
	if decl == nil {
		return nil
	}
	if s.trigger != nil {
		return fmt.Errorf("%s:%d:%d: duplicate trigger declaration (previously declared at %s:%d:%d)",
			decl.GetSpan().Start.File, decl.GetSpan().Start.Line, decl.GetSpan().Start.Column,
			s.trigger.GetSpan().Start.File, s.trigger.GetSpan().Start.Line, s.trigger.GetSpan().Start.Column)
	}
	if _, err := TriggerPolicyFromDecl(decl); err != nil {
		return err
	}
	switch TriggerRouteKindFromDecl(decl) {
	case TriggerRouteKindCapability, TriggerRouteKindIntent:
	default:
		return fmt.Errorf("%s:%d:%d: unsupported trigger route %q",
			decl.GetSpan().Start.File, decl.GetSpan().Start.Line, decl.GetSpan().Start.Column, decl.Policy.Value)
	}
	s.trigger = decl
	return nil
}

func (s *SymbolTable) resolveTypeDecl(decl *TypeDecl) error {
	if decl == nil {
		return nil
	}
	switch body := decl.Body.(type) {
	case *RecordTypeDefinition:
		for _, field := range body.Fields {
			if err := s.resolveTypeExpr(field.Type); err != nil {
				return err
			}
		}
	case *EnumTypeDefinition:
		// Enums are self-contained.
	}
	return nil
}

func (s *SymbolTable) resolveRunLike(agentName string, items []ExecutionItem, span SourceSpan) error {
	if strings.TrimSpace(agentName) == "" {
		return fmt.Errorf("%s:%d:%d: run/delegate target is required", span.Start.File, span.Start.Line, span.Start.Column)
	}
	if _, ok := s.agents[agentName]; !ok {
		return fmt.Errorf("%s:%d:%d: unknown agent %q", span.Start.File, span.Start.Line, span.Start.Column, agentName)
	}
	for _, item := range items {
		if err := s.resolveExecutionItem(item); err != nil {
			return err
		}
	}
	return nil
}

func (s *SymbolTable) resolveExecutionItem(item ExecutionItem) error {
	switch node := item.(type) {
	case *FromClause:
		return s.resolveValueExpr(node.Source)
	case *GoalClause:
		return nil
	case *DirectiveClause:
		for _, arg := range node.Arguments {
			if err := s.resolveValueExpr(arg); err != nil {
				return err
			}
		}
		return nil
	case *DirectiveBlock:
		for _, arg := range node.Arguments {
			if err := s.resolveValueExpr(arg); err != nil {
				return err
			}
		}
		if node.Predicate != nil {
			if err := s.resolvePredicateExpr(*node.Predicate); err != nil {
				return err
			}
		}
		for _, child := range node.Body {
			if err := s.resolveExecutionItem(child); err != nil {
				return err
			}
		}
		return nil
	case *CapabilityInvocation:
		if err := s.resolveCapabilityInvocation(node); err != nil {
			return err
		}
		return nil
	case *CaptureBlock:
		return s.resolveCaptureBlock(node)
	case *RunDecl:
		return s.resolveRunLike(node.Agent.Value, node.Items, node.GetSpan())
	case *DelegateDecl:
		return s.resolveRunLike(node.Agent.Value, node.Items, node.GetSpan())
	case *RouteDecl:
		return s.resolveRouteDecl(node)
	case *AskDecl:
		return s.resolveAskItems(node.Items)
	case *PipelineDecl:
		return s.resolvePipelineDecl(node)
	default:
		return fmt.Errorf("%s:%d:%d: unsupported execution item %T", item.GetSpan().Start.File, item.GetSpan().Start.Line, item.GetSpan().Start.Column, item)
	}
}

func (s *SymbolTable) resolveRouteDecl(decl *RouteDecl) error {
	if decl == nil {
		return nil
	}
	for _, branch := range decl.Branches {
		if !branch.IsElse {
			if err := s.resolvePredicateExpr(branch.Predicate); err != nil {
				return err
			}
		}
		for _, item := range branch.Body {
			if err := s.resolveExecutionItem(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *SymbolTable) resolveAskItems(items []AskItem) error {
	for _, item := range items {
		switch node := item.(type) {
		case *QuestionClause:
			continue
		case *ChoicesListClause:
			for _, entry := range node.Items {
				if err := s.resolveValueExpr(entry); err != nil {
					return err
				}
			}
		case *ChoicesReferenceClause:
			if err := s.resolveValueExpr(node.Source); err != nil {
				return err
			}
		case *CaptureBlock:
			if err := s.resolveCaptureBlock(node); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s:%d:%d: unsupported ask item %T", item.GetSpan().Start.File, item.GetSpan().Start.Line, item.GetSpan().Start.Column, item)
		}
	}
	return nil
}

func (s *SymbolTable) resolvePipelineDecl(decl *PipelineDecl) error {
	if decl == nil {
		return nil
	}
	seen := make(map[string]SourceSpan, len(decl.Stages))
	for _, stage := range decl.Stages {
		name := strings.TrimSpace(stage.Name.Value)
		if name == "" {
			return fmt.Errorf("%s:%d:%d: stage name required", stage.GetSpan().Start.File, stage.GetSpan().Start.Line, stage.GetSpan().Start.Column)
		}
		if prev, exists := seen[name]; exists {
			return fmt.Errorf("%s:%d:%d: duplicate stage %q (previously declared at %s:%d:%d)",
				stage.GetSpan().Start.File, stage.GetSpan().Start.Line, stage.GetSpan().Start.Column, name, prev.Start.File, prev.Start.Line, prev.Start.Column)
		}
		seen[name] = stage.GetSpan()
		for _, item := range stage.Body {
			if err := s.resolveExecutionItem(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *SymbolTable) resolveCaptureBlock(block *CaptureBlock) error {
	if block == nil {
		return nil
	}
	for _, binding := range block.Bindings {
		if err := s.validateReference(binding.Source); err != nil {
			return err
		}
		if binding.Annotation != nil {
			if err := s.resolveTypeExpr(binding.Annotation); err != nil {
				return err
			}
		}
		if err := s.validateReference(binding.Destination); err != nil {
			return err
		}
	}
	return nil
}

func (s *SymbolTable) resolveCapabilityInvocation(inv *CapabilityInvocation) error {
	if inv == nil {
		return nil
	}
	call, err := LowerCapabilityInvocation(inv)
	if err != nil {
		return err
	}
	if s.capability != nil {
		if _, ok := s.capability.Select(call.CapabilityID); !ok {
			if _, ok := s.capability.Select(strings.TrimSpace(inv.Capability.Value)); !ok {
				return fmt.Errorf("%s:%d:%d: unknown capability %q", inv.GetSpan().Start.File, inv.GetSpan().Start.Line, inv.GetSpan().Start.Column, inv.Capability.Value)
			}
		}
		if strings.TrimSpace(inv.Capability.Value) == "" {
			return fmt.Errorf("%s:%d:%d: unknown capability %q", inv.GetSpan().Start.File, inv.GetSpan().Start.Line, inv.GetSpan().Start.Column, inv.Capability.Value)
		}
	}
	if inv.Target != nil {
		if err := s.validateReference(inv.Target); err != nil {
			return err
		}
	}
	if inv.Input != nil {
		if err := s.validateReference(inv.Input); err != nil {
			return err
		}
	}
	s.invocations = append(s.invocations, inv)
	return nil
}

func (s *SymbolTable) resolvePredicateExpr(pred PredicateExpr) error {
	if pred.Kind == "" && pred.Raw == "" {
		return nil
	}
	if pred.Kind == "missing" || pred.Kind == "present" {
		return s.validateReference(pred.Subject)
	}
	if err := s.validateReference(pred.Subject); err != nil {
		return err
	}
	if pred.Value != nil {
		if expr, ok := pred.Value.(ValueExpr); ok {
			if err := s.validateReference(expr); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *SymbolTable) resolveValueExpr(expr ValueExpr) error {
	return s.validateReference(expr)
}

func (s *SymbolTable) validateReference(expr ValueExpr) error {
	switch v := expr.(type) {
	case nil:
		return nil
	case PathExpr:
		return s.validateValueNamespace(&v)
	case *Identifier:
		return s.validateValueNamespace(v)
	case *PathExpr:
		return s.validateValueNamespace(v)
	case Identifier:
		return s.validateValueNamespace(&v)
	case *StringLiteral, *NumberLiteral:
		return nil
	case *ListLiteral:
		for _, entry := range v.Entries {
			if err := s.resolveValueExpr(entry); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%s:%d:%d: unsupported value expression %T", expr.GetSpan().Start.File, expr.GetSpan().Start.Line, expr.GetSpan().Start.Column, expr)
	}
}

func (s *SymbolTable) resolveTypeExpr(expr TypeExpr) error {
	switch v := expr.(type) {
	case nil:
		return nil
	case *NamedTypeExpr:
		name := strings.TrimSpace(v.Name.Raw)
		if isBuiltInType(name) {
			return nil
		}
		if _, ok := s.types[name]; !ok {
			if isLowerLiteral(name) {
				return nil
			}
			return fmt.Errorf("%s:%d:%d: unknown type %q", v.GetSpan().Start.File, v.GetSpan().Start.Line, v.GetSpan().Start.Column, name)
		}
		return nil
	case *ListTypeExpr:
		return s.resolveTypeExpr(v.Element)
	case *OptionalTypeExpr:
		return s.resolveTypeExpr(v.Element)
	case *MapTypeExpr:
		if err := s.resolveTypeExpr(v.Key); err != nil {
			return err
		}
		return s.resolveTypeExpr(v.Value)
	case *UnionTypeExpr:
		for _, option := range v.Options {
			if err := s.resolveTypeExpr(option); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%s:%d:%d: unsupported type expression %T", expr.GetSpan().Start.File, expr.GetSpan().Start.Line, expr.GetSpan().Start.Column, expr)
	}
}

func (s *SymbolTable) validateValueNamespace(expr ValueExpr) error {
	path, ok := valueExprPath(expr)
	if !ok {
		return nil
	}
	if len(path.Parts) < 2 {
		return nil
	}
	return s.validateNamespace(path)
}

func (s *SymbolTable) validateNamespace(expr PathExpr) error {
	if len(expr.Parts) == 0 {
		return fmt.Errorf("%s:%d:%d: empty namespace reference", expr.Span.Start.File, expr.Span.Start.Line, expr.Span.Start.Column)
	}
	namespace := strings.TrimSpace(expr.Parts[0].Value)
	switch namespace {
	case "input":
		if len(expr.Parts) < 2 {
			return fmt.Errorf("%s:%d:%d: input reference requires a name", expr.Span.Start.File, expr.Span.Start.Line, expr.Span.Start.Column)
		}
		if _, ok := s.inputs[expr.Parts[1].Value]; !ok {
			return fmt.Errorf("%s:%d:%d: unknown input %q", expr.Span.Start.File, expr.Span.Start.Line, expr.Span.Start.Column, expr.Parts[1].Value)
		}
	case "state", "scratch", "user", "output":
		if len(expr.Parts) < 2 {
			return fmt.Errorf("%s:%d:%d: %s reference requires a field name", expr.Span.Start.File, expr.Span.Start.Line, expr.Span.Start.Column, namespace)
		}
	default:
		return fmt.Errorf("%s:%d:%d: unknown namespace %q", expr.Span.Start.File, expr.Span.Start.Line, expr.Span.Start.Column, namespace)
	}
	return nil
}

func (s *SymbolTable) validateCapabilityPolicy() error {
	if s == nil || s.Document == nil {
		return nil
	}
	if s.trigger == nil {
		return fmt.Errorf("%s:%d:%d: thoughtrecipe must declare a trigger", s.Document.Header.GetSpan().Start.File, s.Document.Header.GetSpan().Start.Line, s.Document.Header.GetSpan().Start.Column)
	}
	triggerKind := TriggerRouteKindFromDecl(s.trigger)
	triggerPolicy, err := TriggerPolicyFromDecl(s.trigger)
	if err != nil {
		return err
	}
	if len(s.invocations) > 0 && s.capability == nil {
		return fmt.Errorf("%s:%d:%d: capability registry is required to validate capability invocations", s.invocations[0].GetSpan().Start.File, s.invocations[0].GetSpan().Start.Line, s.invocations[0].GetSpan().Start.Column)
	}
	if triggerKind == TriggerRouteKindCapability {
		if hasAskUserDecl(s.Document) {
			if !triggerPolicy.AskUser {
				return fmt.Errorf("%s:%d:%d: ask user blocks require trigger policy line 'may ask user'", s.trigger.GetSpan().Start.File, s.trigger.GetSpan().Start.Line, s.trigger.GetSpan().Start.Column)
			}
		}
		for _, inv := range s.invocations {
			desc, ok := s.lookupCapability(inv)
			if !ok {
				continue
			}
			required := CapabilityRequirementsFromDescriptor(desc)
			if !CapabilityRequirementsSatisfied(triggerPolicy, required) {
				return fmt.Errorf("%s:%d:%d: capability %q requires trigger policy allowing read workspace%s",
					inv.GetSpan().Start.File, inv.GetSpan().Start.Line, inv.GetSpan().Start.Column, inv.Capability.Value, writeSuffix(required))
			}
		}
	}
	return nil
}

func (s *SymbolTable) lookupCapability(inv *CapabilityInvocation) (core.CapabilityDescriptor, bool) {
	if s == nil || s.capability == nil || inv == nil {
		return core.CapabilityDescriptor{}, false
	}
	capID := NormalizeCapabilityReference(inv.Capability.Value)
	if capID != "" {
		if desc, ok := s.capability.Select(capID); ok {
			return desc, true
		}
	}
	if desc, ok := s.capability.Select(strings.TrimSpace(inv.Capability.Value)); ok {
		return desc, true
	}
	return core.CapabilityDescriptor{}, false
}

func hasAskUserDecl(doc *ThoughtRecipeDocument) bool {
	if doc == nil {
		return false
	}
	for _, decl := range doc.Declarations {
		if _, ok := decl.(*AskDecl); ok {
			return true
		}
	}
	return false
}

func writeSuffix(req TriggerPolicyRequirements) string {
	if req.WriteWorkspace {
		return " and write workspace"
	}
	return ""
}

func validateAgentParadigm(agent *AgentDecl) error {
	if agent == nil {
		return nil
	}
	paradigm := strings.TrimSpace(agent.AgentType.Value)
	if paradigm == "" {
		return fmt.Errorf("%s:%d:%d: agent type is required", agent.GetSpan().Start.File, agent.GetSpan().Start.Line, agent.GetSpan().Start.Column)
	}
	if !isSupportedAgentParadigm(paradigm) {
		return fmt.Errorf("%s:%d:%d: unsupported agent paradigm %q", agent.GetSpan().Start.File, agent.GetSpan().Start.Line, agent.GetSpan().Start.Column, paradigm)
	}
	return nil
}

func valueExprPath(expr ValueExpr) (PathExpr, bool) {
	switch v := expr.(type) {
	case *PathExpr:
		return *v, true
	case PathExpr:
		return v, true
	case *Identifier:
		return PathExpr{positioned: v.positioned, Raw: v.Value, Parts: []Identifier{*v}}, true
	case Identifier:
		return PathExpr{positioned: v.positioned, Raw: v.Value, Parts: []Identifier{v}}, true
	default:
		return PathExpr{}, false
	}
}

func isBuiltInType(name string) bool {
	switch strings.TrimSpace(name) {
	case "Text", "Markdown", "Json", "Bool", "Number", "Percent", "File", "Path", "Workspace", "Diff":
		return true
	default:
		return false
	}
}

func isLowerLiteral(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, r := range name {
		if r >= 'A' && r <= 'Z' {
			return false
		}
	}
	return true
}
