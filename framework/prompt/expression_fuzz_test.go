package prompt

import "testing"

// FuzzExpression fuzzes the expression parser with arbitrary input.
// Run with: go test -fuzz=FuzzExpression -fuzztime=30s ./framework/prompt/
func FuzzExpression(f *testing.F) {
	// Seed corpus with valid expressions.
	seeds := []string{
		"true",
		"false",
		"connected",
		`phase == "verify"`,
		"count > 5",
		"a && b",
		"a || b",
		"x exists",
		"(a || b) && c",
		`react.phase == "plan"`,
		"score >= 0",
		"score <= 100",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, expr string) {
		// Must not panic.
		e, err := compileExpression(expr)
		if err != nil {
			return // parse errors are expected
		}
		// Must not panic on evaluation.
		state := map[string]any{
			"a": true, "b": false, "c": true,
			"connected": true, "phase": "verify",
			"count": 5, "score": 42,
			"react": map[string]any{"phase": "plan"},
		}
		_, _ = e.Evaluate(state)
	})
}
