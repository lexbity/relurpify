package thoughtrecipe

import (
	"fmt"
	"strings"
)

// TypeCheckResult reports structural compatibility between two type expressions.
type TypeCheckResult struct {
	OK     bool
	Reason string
}

// TypeSystem validates thoughtrecipe-local structural types and typed captures.
type TypeSystem struct {
	Document *ThoughtRecipeDocument
	types    map[string]*TypeDecl
}

// NewTypeSystem creates a new type system for a document.
func NewTypeSystem(doc *ThoughtRecipeDocument) *TypeSystem {
	ts := &TypeSystem{
		Document: doc,
		types:    make(map[string]*TypeDecl),
	}
	if doc != nil {
		for _, decl := range doc.Declarations {
			if td, ok := decl.(*TypeDecl); ok {
				name := strings.TrimSpace(td.Name.Value)
				if name != "" {
					ts.types[name] = td
				}
			}
		}
	}
	return ts
}

// Validate validates all structural types and typed capture bindings in the document.
func (ts *TypeSystem) Validate() error {
	if ts == nil {
		return fmt.Errorf("type system is nil")
	}
	if ts.Document == nil {
		return fmt.Errorf("thoughtrecipe document is nil")
	}
	for _, decl := range ts.Document.Declarations {
		switch node := decl.(type) {
		case *TypeDecl:
			if err := ts.validateTypeDecl(node); err != nil {
				return err
			}
		case *RunDecl:
			if err := ts.validateExecutionItems(node.Items); err != nil {
				return err
			}
		case *DelegateDecl:
			if err := ts.validateExecutionItems(node.Items); err != nil {
				return err
			}
		case *RouteDecl:
			for _, branch := range node.Branches {
				if err := ts.validateExecutionItems(branch.Body); err != nil {
					return err
				}
			}
		case *AskDecl:
			if err := ts.validateAskItems(node.Items); err != nil {
				return err
			}
		case *PipelineDecl:
			for _, stage := range node.Stages {
				if err := ts.validateExecutionItems(stage.Body); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ValidateCaptureBinding validates a single capture binding.
func (ts *TypeSystem) ValidateCaptureBinding(binding CaptureBinding) error {
	if binding.Annotation != nil {
		if err := ts.validateTypeExpr(binding.Annotation, false); err != nil {
			return err
		}
	}
	if _, ok := validateCaptureDestinationPath(binding.Destination); !ok {
		return fmt.Errorf("%s:%d:%d: capture destination must use state, scratch, user, or output", binding.GetSpan().Start.File, binding.GetSpan().Start.Line, binding.GetSpan().Start.Column)
	}
	if binding.Forwarding {
		if _, ok := valueExprPath(binding.Source); !ok {
			return fmt.Errorf("%s:%d:%d: direct forwarding requires a reference source", binding.GetSpan().Start.File, binding.GetSpan().Start.Line, binding.GetSpan().Start.Column)
		}
	}
	return nil
}

// Compatible reports whether two type expressions have the same structural shape.
func (ts *TypeSystem) Compatible(expected, actual TypeExpr) TypeCheckResult {
	if err := ts.validateTypeExpr(expected, false); err != nil {
		return TypeCheckResult{Reason: err.Error()}
	}
	if err := ts.validateTypeExpr(actual, false); err != nil {
		return TypeCheckResult{Reason: err.Error()}
	}
	esig, err := ts.typeSignature(expected, false)
	if err != nil {
		return TypeCheckResult{Reason: err.Error()}
	}
	asig, err := ts.typeSignature(actual, false)
	if err != nil {
		return TypeCheckResult{Reason: err.Error()}
	}
	if esig != asig {
		return TypeCheckResult{Reason: fmt.Sprintf("type mismatch: expected %s, got %s", esig, asig)}
	}
	return TypeCheckResult{OK: true}
}

func (ts *TypeSystem) validateTypeDecl(decl *TypeDecl) error {
	if decl == nil || decl.Body == nil {
		return nil
	}
	switch body := decl.Body.(type) {
	case *RecordTypeDefinition:
		for _, field := range body.Fields {
			if err := ts.validateTypeExpr(field.Type, false); err != nil {
				return err
			}
		}
	case *EnumTypeDefinition:
		for _, value := range body.Values {
			if !isEnumLiteralName(value.Value) {
				return fmt.Errorf("%s:%d:%d: enum value %q must be a literal", value.GetSpan().Start.File, value.GetSpan().Start.Line, value.GetSpan().Start.Column, value.Value)
			}
		}
	}
	return nil
}

func (ts *TypeSystem) validateExecutionItems(items []ExecutionItem) error {
	for _, item := range items {
		switch node := item.(type) {
		case *DirectiveBlock:
			if node.Predicate != nil {
				if err := ts.validatePredicate(node.Predicate); err != nil {
					return err
				}
			}
			if err := ts.validateExecutionItems(node.Body); err != nil {
				return err
			}
		case *CaptureBlock:
			for _, binding := range node.Bindings {
				if err := ts.ValidateCaptureBinding(binding); err != nil {
					return err
				}
			}
		case *RunDecl:
			if err := ts.validateExecutionItems(node.Items); err != nil {
				return err
			}
		case *DelegateDecl:
			if err := ts.validateExecutionItems(node.Items); err != nil {
				return err
			}
		case *RouteDecl:
			for _, branch := range node.Branches {
				if err := ts.validateExecutionItems(branch.Body); err != nil {
					return err
				}
			}
		case *AskDecl:
			if err := ts.validateAskItems(node.Items); err != nil {
				return err
			}
		case *PipelineDecl:
			for _, stage := range node.Stages {
				if err := ts.validateExecutionItems(stage.Body); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (ts *TypeSystem) validateAskItems(items []AskItem) error {
	for _, item := range items {
		switch node := item.(type) {
		case *ChoicesListClause:
			for _, entry := range node.Items {
				if err := ts.validateValueExpr(entry); err != nil {
					return err
				}
			}
		case *ChoicesReferenceClause:
			if err := ts.validateValueExpr(node.Source); err != nil {
				return err
			}
		case *CaptureBlock:
			for _, binding := range node.Bindings {
				if err := ts.ValidateCaptureBinding(binding); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (ts *TypeSystem) validatePredicate(pred *PredicateExpr) error {
	if pred == nil {
		return nil
	}
	if pred.Kind == "missing" || pred.Kind == "present" || pred.Kind == "is" || pred.Kind == "contains" {
		return ts.validateValueExpr(&pred.Subject)
	}
	if pred.Kind == "confidence_below" {
		if pred.Value != nil {
			return ts.validateValueExpr(pred.Value)
		}
	}
	return nil
}

func (ts *TypeSystem) validateValueExpr(expr ValueExpr) error {
	stack := []ValueExpr{expr}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch v := current.(type) {
		case nil:
			continue
		case PathExpr:
			if err := ts.validateReferencePath(v); err != nil {
				return err
			}
		case *PathExpr:
			if err := ts.validateReferencePath(*v); err != nil {
				return err
			}
		case Identifier, *Identifier, StringLiteral, *StringLiteral, NumberLiteral, *NumberLiteral:
			continue
		case ListLiteral:
			for i := len(v.Entries) - 1; i >= 0; i-- {
				stack = append(stack, v.Entries[i])
			}
		case *ListLiteral:
			for i := len(v.Entries) - 1; i >= 0; i-- {
				stack = append(stack, v.Entries[i])
			}
		default:
			if current == nil {
				continue
			}
			return fmt.Errorf("%s:%d:%d: unsupported value expression %T", current.GetSpan().Start.File, current.GetSpan().Start.Line, current.GetSpan().Start.Column, current)
		}
	}
	return nil
}

func (ts *TypeSystem) validateReferencePath(path PathExpr) error {
	if len(path.Parts) < 2 {
		return nil
	}
	switch strings.TrimSpace(path.Parts[0].Value) {
	case "input", "state", "scratch", "user", "output":
		return nil
	default:
		return fmt.Errorf("%s:%d:%d: unknown namespace %q", path.Span.Start.File, path.Span.Start.Line, path.Span.Start.Column, path.Parts[0].Value)
	}
}

func (ts *TypeSystem) validateTypeExpr(expr TypeExpr, enumContext bool) error {
	type typeExprFrame struct {
		expr        TypeExpr
		enumContext bool
	}

	stack := []typeExprFrame{{expr: expr, enumContext: enumContext}}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch v := current.expr.(type) {
		case nil:
			continue
		case *NamedTypeExpr:
			name := strings.TrimSpace(v.Name.Raw)
			if name == "" {
				return fmt.Errorf("%s:%d:%d: empty type reference", v.GetSpan().Start.File, v.GetSpan().Start.Line, v.GetSpan().Start.Column)
			}
			if isBuiltInType(name) || ts.hasDeclaredType(name) {
				continue
			}
			if current.enumContext && isEnumLiteralName(name) {
				continue
			}
			return fmt.Errorf("%s:%d:%d: unknown type %q", v.GetSpan().Start.File, v.GetSpan().Start.Line, v.GetSpan().Start.Column, name)
		case NamedTypeExpr:
			stack = append(stack, typeExprFrame{expr: &v, enumContext: current.enumContext})
		case *ListTypeExpr:
			stack = append(stack, typeExprFrame{expr: v.Element, enumContext: false})
		case ListTypeExpr:
			stack = append(stack, typeExprFrame{expr: v.Element, enumContext: false})
		case *OptionalTypeExpr:
			stack = append(stack, typeExprFrame{expr: v.Element, enumContext: false})
		case OptionalTypeExpr:
			stack = append(stack, typeExprFrame{expr: v.Element, enumContext: false})
		case *MapTypeExpr:
			stack = append(stack, typeExprFrame{expr: v.Value, enumContext: false})
			stack = append(stack, typeExprFrame{expr: v.Key, enumContext: false})
		case MapTypeExpr:
			stack = append(stack, typeExprFrame{expr: v.Value, enumContext: false})
			stack = append(stack, typeExprFrame{expr: v.Key, enumContext: false})
		case *UnionTypeExpr:
			if len(v.Options) == 0 {
				return fmt.Errorf("%s:%d:%d: empty union type", v.GetSpan().Start.File, v.GetSpan().Start.Line, v.GetSpan().Start.Column)
			}
			for i := len(v.Options) - 1; i >= 0; i-- {
				stack = append(stack, typeExprFrame{expr: v.Options[i], enumContext: true})
			}
		case UnionTypeExpr:
			stack = append(stack, typeExprFrame{expr: &v, enumContext: current.enumContext})
		default:
			return fmt.Errorf("%s:%d:%d: unsupported type expression %T", current.expr.GetSpan().Start.File, current.expr.GetSpan().Start.Line, current.expr.GetSpan().Start.Column, current.expr)
		}
	}
	return nil
}

func (ts *TypeSystem) typeSignature(expr TypeExpr, enumContext bool) (string, error) {
	type typeSigFrame struct {
		expr        TypeExpr
		enumContext bool
		phase       int
		childCount  int
	}

	popResult := func(results *[]string, count int) []string {
		if count == 0 {
			return nil
		}
		start := len(*results) - count
		out := make([]string, count)
		copy(out, (*results)[start:])
		*results = (*results)[:start]
		return out
	}

	frameStack := []typeSigFrame{{expr: expr, enumContext: enumContext}}
	results := make([]string, 0, 8)
	for len(frameStack) > 0 {
		current := frameStack[len(frameStack)-1]
		frameStack = frameStack[:len(frameStack)-1]

		switch v := current.expr.(type) {
		case nil:
			return "", fmt.Errorf("type expression is nil")
		case *NamedTypeExpr:
			name := strings.TrimSpace(v.Name.Raw)
			if name == "" {
				return "", fmt.Errorf("%s:%d:%d: empty type reference", v.GetSpan().Start.File, v.GetSpan().Start.Line, v.GetSpan().Start.Column)
			}
			if current.enumContext && isEnumLiteralName(name) {
				results = append(results, "enum:"+name)
			} else {
				results = append(results, name)
			}
		case NamedTypeExpr:
			frameStack = append(frameStack, typeSigFrame{expr: &v, enumContext: current.enumContext})
		case *ListTypeExpr:
			if current.phase == 0 {
				frameStack = append(frameStack, typeSigFrame{expr: v, enumContext: current.enumContext, phase: 1, childCount: 1})
				frameStack = append(frameStack, typeSigFrame{expr: v.Element, enumContext: false})
				continue
			}
			children := popResult(&results, current.childCount)
			results = append(results, "list<"+children[0]+">")
		case ListTypeExpr:
			frameStack = append(frameStack, typeSigFrame{expr: &v, enumContext: current.enumContext})
		case *OptionalTypeExpr:
			if current.phase == 0 {
				frameStack = append(frameStack, typeSigFrame{expr: v, enumContext: current.enumContext, phase: 1, childCount: 1})
				frameStack = append(frameStack, typeSigFrame{expr: v.Element, enumContext: false})
				continue
			}
			children := popResult(&results, current.childCount)
			results = append(results, "optional<"+children[0]+">")
		case OptionalTypeExpr:
			frameStack = append(frameStack, typeSigFrame{expr: &v, enumContext: current.enumContext})
		case *MapTypeExpr:
			if current.phase == 0 {
				frameStack = append(frameStack, typeSigFrame{expr: v, enumContext: current.enumContext, phase: 1, childCount: 2})
				frameStack = append(frameStack, typeSigFrame{expr: v.Value, enumContext: false})
				frameStack = append(frameStack, typeSigFrame{expr: v.Key, enumContext: false})
				continue
			}
			children := popResult(&results, current.childCount)
			results = append(results, "map<"+children[0]+","+children[1]+">")
		case MapTypeExpr:
			frameStack = append(frameStack, typeSigFrame{expr: &v, enumContext: current.enumContext})
		case *UnionTypeExpr:
			if len(v.Options) == 0 {
				return "", fmt.Errorf("%s:%d:%d: empty union type", v.GetSpan().Start.File, v.GetSpan().Start.Line, v.GetSpan().Start.Column)
			}
			if current.phase == 0 {
				frameStack = append(frameStack, typeSigFrame{expr: v, enumContext: current.enumContext, phase: 1, childCount: len(v.Options)})
				for i := len(v.Options) - 1; i >= 0; i-- {
					frameStack = append(frameStack, typeSigFrame{expr: v.Options[i], enumContext: true})
				}
				continue
			}
			children := popResult(&results, current.childCount)
			results = append(results, "union<"+strings.Join(children, "|")+">")
		case UnionTypeExpr:
			frameStack = append(frameStack, typeSigFrame{expr: &v, enumContext: current.enumContext})
		default:
			return "", fmt.Errorf("%s:%d:%d: unsupported type expression %T", current.expr.GetSpan().Start.File, current.expr.GetSpan().Start.Line, current.expr.GetSpan().Start.Column, current.expr)
		}
	}
	if len(results) != 1 {
		if len(results) == 0 {
			return "", fmt.Errorf("type signature produced no result")
		}
		return strings.Join(results, ","), nil
	}
	return results[0], nil
}

func (ts *TypeSystem) hasDeclaredType(name string) bool {
	if ts == nil {
		return false
	}
	_, ok := ts.types[strings.TrimSpace(name)]
	return ok
}

func isEnumLiteralName(name string) bool {
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
