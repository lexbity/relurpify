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
	switch v := expr.(type) {
	case nil:
		return nil
	case PathExpr:
		return ts.validateReferencePath(v)
	case *PathExpr:
		return ts.validateReferencePath(*v)
	case Identifier:
		return nil
	case *Identifier:
		return nil
	case StringLiteral, *StringLiteral, NumberLiteral, *NumberLiteral:
		return nil
	case ListLiteral:
		for _, entry := range v.Entries {
			if err := ts.validateValueExpr(entry); err != nil {
				return err
			}
		}
		return nil
	case *ListLiteral:
		for _, entry := range v.Entries {
			if err := ts.validateValueExpr(entry); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%s:%d:%d: unsupported value expression %T", expr.GetSpan().Start.File, expr.GetSpan().Start.Line, expr.GetSpan().Start.Column, expr)
	}
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
	switch v := expr.(type) {
	case nil:
		return nil
	case *NamedTypeExpr:
		name := strings.TrimSpace(v.Name.Raw)
		if name == "" {
			return fmt.Errorf("%s:%d:%d: empty type reference", v.GetSpan().Start.File, v.GetSpan().Start.Line, v.GetSpan().Start.Column)
		}
		if isBuiltInType(name) || ts.hasDeclaredType(name) {
			return nil
		}
		if enumContext && isEnumLiteralName(name) {
			return nil
		}
		return fmt.Errorf("%s:%d:%d: unknown type %q", v.GetSpan().Start.File, v.GetSpan().Start.Line, v.GetSpan().Start.Column, name)
	case NamedTypeExpr:
		return ts.validateTypeExpr(&v, enumContext)
	case *ListTypeExpr:
		return ts.validateTypeExpr(v.Element, false)
	case ListTypeExpr:
		return ts.validateTypeExpr(v.Element, false)
	case *OptionalTypeExpr:
		return ts.validateTypeExpr(v.Element, false)
	case OptionalTypeExpr:
		return ts.validateTypeExpr(v.Element, false)
	case *MapTypeExpr:
		if err := ts.validateTypeExpr(v.Key, false); err != nil {
			return err
		}
		return ts.validateTypeExpr(v.Value, false)
	case MapTypeExpr:
		if err := ts.validateTypeExpr(v.Key, false); err != nil {
			return err
		}
		return ts.validateTypeExpr(v.Value, false)
	case *UnionTypeExpr:
		if len(v.Options) == 0 {
			return fmt.Errorf("%s:%d:%d: empty union type", v.GetSpan().Start.File, v.GetSpan().Start.Line, v.GetSpan().Start.Column)
		}
		for _, opt := range v.Options {
			if err := ts.validateTypeExpr(opt, true); err != nil {
				return err
			}
		}
		return nil
	case UnionTypeExpr:
		return ts.validateTypeExpr(&v, enumContext)
	default:
		return fmt.Errorf("%s:%d:%d: unsupported type expression %T", expr.GetSpan().Start.File, expr.GetSpan().Start.Line, expr.GetSpan().Start.Column, expr)
	}
}

func (ts *TypeSystem) typeSignature(expr TypeExpr, enumContext bool) (string, error) {
	switch v := expr.(type) {
	case *NamedTypeExpr:
		name := strings.TrimSpace(v.Name.Raw)
		if name == "" {
			return "", fmt.Errorf("%s:%d:%d: empty type reference", v.GetSpan().Start.File, v.GetSpan().Start.Line, v.GetSpan().Start.Column)
		}
		if enumContext && isEnumLiteralName(name) {
			return "enum:" + name, nil
		}
		return name, nil
	case NamedTypeExpr:
		return ts.typeSignature(&v, enumContext)
	case *ListTypeExpr:
		elem, err := ts.typeSignature(v.Element, false)
		if err != nil {
			return "", err
		}
		return "list<" + elem + ">", nil
	case ListTypeExpr:
		return ts.typeSignature(&v, enumContext)
	case *OptionalTypeExpr:
		elem, err := ts.typeSignature(v.Element, false)
		if err != nil {
			return "", err
		}
		return "optional<" + elem + ">", nil
	case OptionalTypeExpr:
		return ts.typeSignature(&v, enumContext)
	case *MapTypeExpr:
		key, err := ts.typeSignature(v.Key, false)
		if err != nil {
			return "", err
		}
		val, err := ts.typeSignature(v.Value, false)
		if err != nil {
			return "", err
		}
		return "map<" + key + "," + val + ">", nil
	case MapTypeExpr:
		return ts.typeSignature(&v, enumContext)
	case *UnionTypeExpr:
		if len(v.Options) == 0 {
			return "", fmt.Errorf("%s:%d:%d: empty union type", v.GetSpan().Start.File, v.GetSpan().Start.Line, v.GetSpan().Start.Column)
		}
		parts := make([]string, 0, len(v.Options))
		for _, opt := range v.Options {
			sig, err := ts.typeSignature(opt, true)
			if err != nil {
				return "", err
			}
			parts = append(parts, sig)
		}
		return "union<" + strings.Join(parts, "|") + ">", nil
	case UnionTypeExpr:
		return ts.typeSignature(&v, enumContext)
	default:
		return "", fmt.Errorf("%s:%d:%d: unsupported type expression %T", expr.GetSpan().Start.File, expr.GetSpan().Start.Line, expr.GetSpan().Start.Column, expr)
	}
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
