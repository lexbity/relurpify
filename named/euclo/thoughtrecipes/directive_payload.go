package thoughtrecipe

import "fmt"

// directive_payload.go is the only permitted home for direct indexing into
// TypedDirective slices carried on ExecutionStep. Node code resolves directive
// payloads through these accessors instead of hand-parsing strings or indexing
// `step.Directives[i]` directly (enforced by the phase's grep gate).
//
// The accessors match the grammar-level contract vocabulary: text arguments,
// the optional `when` predicate, and nested block directives.

// Has reports whether the directive list contains a directive with the exact
// name.
func Has(directives []TypedDirective, name string) bool {
	for _, directive := range directives {
		if directive.Name == name {
			return true
		}
	}
	return false
}

// DirectiveText returns the text arguments of the first directive with the
// given name, in source order. Nil when the directive is absent.
func DirectiveText(directives []TypedDirective, name string) []string {
	for _, directive := range directives {
		if directive.Name == name {
			return directive.TextArgs
		}
	}
	return nil
}

// DirectivePredicate returns the `when` predicate of the first directive with
// the given name. Nil when the directive is absent or carries no predicate.
func DirectivePredicate(directives []TypedDirective, name string) *PredicateExpr {
	for _, directive := range directives {
		if directive.Name == name {
			return directive.Predicate
		}
	}
	return nil
}

// ExactlyOne returns the single directive with the given name, or an error
// when it is absent or appears more than once. It is the cardinality helper
// the paradigm option builders use for required-once directives.
func ExactlyOne(directives []TypedDirective, name string) (TypedDirective, error) {
	var found TypedDirective
	count := 0
	for _, directive := range directives {
		if directive.Name != name {
			continue
		}
		count++
		if count == 1 {
			found = directive
			continue
		}
		return TypedDirective{}, fmt.Errorf("duplicate directive %q at line %d", name, directive.Span.Start.Line)
	}
	if count == 0 {
		return TypedDirective{}, fmt.Errorf("missing required directive %q", name)
	}
	return found, nil
}

// AtMostOne returns the single directive with the given name and whether it
// was present, or an error when it appears more than once. It is the
// cardinality helper for optional-once directives.
func AtMostOne(directives []TypedDirective, name string) (TypedDirective, bool, error) {
	var found TypedDirective
	count := 0
	for _, directive := range directives {
		if directive.Name != name {
			continue
		}
		count++
		if count == 1 {
			found = directive
			continue
		}
		return TypedDirective{}, false, fmt.Errorf("duplicate directive %q at line %d", name, directive.Span.Start.Line)
	}
	if count == 0 {
		return TypedDirective{}, false, nil
	}
	return found, true, nil
}

// StepItems returns every directive with the given name, in source order. It
// operates on any directive slice: the top-level directives of a step, or the
// nested Body of a block (where it collects ordered children such as repeated
// `task`/`do` items).
func StepItems(directives []TypedDirective, name string) []TypedDirective {
	var out []TypedDirective
	for _, directive := range directives {
		if directive.Name == name {
			out = append(out, directive)
		}
	}
	return out
}

// DirectiveNames returns the top-level directive names in source order. It is
// the projection used by telemetry and step metadata: execution_directives is
// the flat name list of the clauses/blocks carried on the step, not raw source
// text and not nested directive content (nested directives are reached through
// their parent's Body).
func DirectiveNames(directives []TypedDirective) []string {
	if len(directives) == 0 {
		return nil
	}
	names := make([]string, 0, len(directives))
	for _, directive := range directives {
		names = append(names, directive.Name)
	}
	return names
}
