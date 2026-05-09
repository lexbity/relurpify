package thoughtrecipe

import (
	"strings"
	"testing"
)

func TestTypeSystemValidatesTypedCapturesAndStructuralTypes(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe review
"Review the code."

trigger as capability:
  may read workspace

input workspace: "**/*.go"
input prompt: user.request

type Risk:
  title: Text
  severity: low | medium | high
  evidence: list<Text>

type ReviewFindings:
  summary: Markdown
  risks: list<Risk>

agent reviewer uses react

run reviewer:
  from input.workspace
  from input.prompt
  goal "Review the code."
  capture:
    findings: ReviewFindings -> state.findings
    result: Markdown -> output.result
    state.findings -> state.plan
`)

	ts := NewTypeSystem(doc)
	if err := ts.Validate(); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	run := doc.Declarations[6].(*RunDecl)
	binding := run.Items[3].(*CaptureBlock).Bindings[2]
	if !binding.Forwarding {
		t.Fatal("expected direct forwarding binding to be marked as forwarding")
	}
	if err := ts.ValidateCaptureBinding(binding); err != nil {
		t.Fatalf("ValidateCaptureBinding failed: %v", err)
	}
}

func TestTypeSystemRejectsUnknownCaptureType(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe review
"Review."

trigger as capability:
  may read workspace

input workspace: "**/*.go"
agent reviewer uses react

run reviewer:
  from input.workspace
  goal "Review the code."
  capture:
    findings: UnknownType -> state.findings
`)

	err := NewTypeSystem(doc).Validate()
	if err == nil {
		t.Fatal("expected unknown capture type error")
	}
}

func TestTypeSystemRejectsUnknownNestedType(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe review
"Review."

trigger as capability:
  may read workspace

input workspace: "**/*.go"
type ReviewFindings:
  summary: Markdown
  risks: list<MissingType>
agent reviewer uses react
`)

	err := NewTypeSystem(doc).Validate()
	if err == nil {
		t.Fatal("expected nested type error")
	}
}

func TestTypeSystemCompatibility(t *testing.T) {
	ts := NewTypeSystem(&ThoughtRecipeDocument{})

	cases := []struct {
		name string
		a    TypeExpr
		b    TypeExpr
		ok   bool
	}{
		{
			name: "same list",
			a:    &ListTypeExpr{Element: &NamedTypeExpr{Name: PathExpr{Raw: "Text"}}},
			b:    &ListTypeExpr{Element: &NamedTypeExpr{Name: PathExpr{Raw: "Text"}}},
			ok:   true,
		},
		{
			name: "different list",
			a:    &ListTypeExpr{Element: &NamedTypeExpr{Name: PathExpr{Raw: "Text"}}},
			b:    &ListTypeExpr{Element: &NamedTypeExpr{Name: PathExpr{Raw: "Markdown"}}},
			ok:   false,
		},
		{
			name: "optional",
			a:    &OptionalTypeExpr{Element: &NamedTypeExpr{Name: PathExpr{Raw: "Text"}}},
			b:    &OptionalTypeExpr{Element: &NamedTypeExpr{Name: PathExpr{Raw: "Text"}}},
			ok:   true,
		},
		{
			name: "map",
			a:    &MapTypeExpr{Key: &NamedTypeExpr{Name: PathExpr{Raw: "Text"}}, Value: &NamedTypeExpr{Name: PathExpr{Raw: "Number"}}},
			b:    &MapTypeExpr{Key: &NamedTypeExpr{Name: PathExpr{Raw: "Text"}}, Value: &NamedTypeExpr{Name: PathExpr{Raw: "Number"}}},
			ok:   true,
		},
	}

	for _, tc := range cases {
		got := ts.Compatible(tc.a, tc.b)
		if got.OK != tc.ok {
			t.Fatalf("%s: compatibility = %v (%s), want %v", tc.name, got.OK, got.Reason, tc.ok)
		}
	}
}

func TestTypeSystemHandlesDeepNestingIteratively(t *testing.T) {
	ts := NewTypeSystem(&ThoughtRecipeDocument{})

	deepType := nestedListType(2000)
	if err := ts.validateTypeExpr(deepType, false); err != nil {
		t.Fatalf("validateTypeExpr failed: %v", err)
	}
	sig, err := ts.typeSignature(deepType, false)
	if err != nil {
		t.Fatalf("typeSignature failed: %v", err)
	}
	if got := strings.Count(sig, "list<"); got != 2000 {
		t.Fatalf("list nesting count = %d, want 2000", got)
	}
	if got := strings.Count(sig, ">"); got != 2000 {
		t.Fatalf("closing bracket count = %d, want 2000", got)
	}

	deepValue := nestedListValue(2000)
	if err := ts.validateValueExpr(deepValue); err != nil {
		t.Fatalf("validateValueExpr failed: %v", err)
	}
}

func nestedListType(depth int) TypeExpr {
	var expr TypeExpr = &NamedTypeExpr{Name: PathExpr{Raw: "Text"}}
	for i := 0; i < depth; i++ {
		expr = &ListTypeExpr{Element: expr}
	}
	return expr
}

func nestedListValue(depth int) ValueExpr {
	var expr ValueExpr = &StringLiteral{Raw: `"leaf"`, Value: "leaf"}
	for i := 0; i < depth; i++ {
		expr = &ListLiteral{Entries: []ValueExpr{expr}}
	}
	return expr
}
