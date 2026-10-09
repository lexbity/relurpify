package arch

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/tooling/arch/gatescan"
)

// ForbiddenLanguage lists vocabulary that marks a compatibility layer. A shim,
// a compat alias, or a stub is half a migration left in the tree: callers keep
// depending on it and the owning code never gets fixed. Matching is
// case-insensitive and substring-based, mirroring the grep gate this check
// replaces — a word boundary would let "Shim-" and "shimmy" through.
//
// The short spelling "compat" is deliberately absent from this list. As a
// substring it cannot be told apart from the project's canonical provider
// vocabulary (package platform/llm/openaicompat, config kind
// "openai_compatible", capability ID euclo:cap.api_compat), and
// check-no-ghost-providers bans exactly those short spellings. It is matched
// on word boundaries instead — see compatWord — which keeps "compat alias" a
// violation while "api_compat" names a capability.
var ForbiddenLanguage = []string{ //nolint:gochecknoglobals // immutable forbidden-vocabulary table
	"shim",
	"compatibility",
	"stub",
	"backward compatibility",
}

// compatWord matches the short spelling "compat" only as a whole word, so
// "compat layer" is a violation while "api_compat", "openaicompat" and
// "openai_compatible" stay the legitimate vocabulary they are. Hyphens make a
// boundary, so "back-compat" is a violation too: that is a layer described in
// shorthand.
var compatWord = regexp.MustCompile(`\bcompat\b`)

// CheckShimLanguage reports string literals in files not covered by exempt
// that carry forbidden architecture language. Concatenated strings
// ("sh" + "im") are binary expressions and are not literals, so they are not
// reported here — the grep gate in grep-architecture-gates remains the
// secondary layer that catches them.
func CheckShimLanguage(files []gatescan.File, exempt Exemption) []string {
	var violations []string
	for _, f := range files {
		if exempt.Covers(f.Path) {
			continue
		}
		ast.Inspect(f.AST, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if pattern := forbiddenIn(gatescan.LiteralValue(lit)); pattern != "" {
				violations = append(violations, fmt.Sprintf(
					"shim: %s carries forbidden language %q in a string literal",
					gatescan.Position(f, lit), pattern))
			}
			return true
		})
	}
	sort.Strings(violations)
	return violations
}

// forbiddenIn reports which forbidden pattern a literal carries, or "" when it
// carries none. Matching is case-insensitive.
func forbiddenIn(value string) string {
	value = strings.ToLower(value)
	for _, pattern := range ForbiddenLanguage {
		if strings.Contains(value, pattern) {
			return pattern
		}
	}
	if compatWord.MatchString(value) {
		return "compat"
	}
	return ""
}
