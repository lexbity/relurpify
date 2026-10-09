package thoughtrecipe

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
