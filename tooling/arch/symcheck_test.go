package arch

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/tooling/arch/gatescan"
)

func TestCheckRemovedSymbolsFlagsEveryPosition(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "call",
			src: `package a

func f() { InvokeOnBestNode(nil) }
`,
			want: "InvokeOnBestNode",
		},
		{
			name: "type position",
			src: `package a

var _ RateLimiter
`,
			want: "RateLimiter",
		},
		{
			name: "selector",
			src: `package a

func f(r *runner) { r.LoadTape() }
`,
			want: "LoadTape",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := []gatescan.File{parse(t, "named/euclo/a.go", tc.src)}
			got := CheckRemovedSymbols(files, Exemption{})
			if len(got) != 1 {
				t.Fatalf("want 1 violation, got %v", got)
			}
			if !strings.Contains(got[0], tc.want) {
				t.Errorf("violation %q should name %q", got[0], tc.want)
			}
			if !strings.HasPrefix(got[0], "dead: named/euclo/a.go:") {
				t.Errorf("violation %q should carry file:line", got[0])
			}
		})
	}
}

func TestCheckRemovedSymbolsIgnoresRenameAdjacentIdentifiers(t *testing.T) {
	files := []gatescan.File{parse(t, "a.go", `package a

type CompiledStepExtra struct{}

func f() { CompiledStepExtra{} }
`)}

	if got := CheckRemovedSymbols(files, Exemption{}); len(got) != 0 {
		t.Fatalf("a different identifier is not the removed symbol, got %v", got)
	}
}

// TestCheckRemovedSymbolsCoversIdentifiersNotStrings records the layering: the
// AST gate reads identifiers, and a removed symbol smuggled through as a string
// literal is caught by the no-dead grep gate in the Makefile.
func TestCheckRemovedSymbolsCoversIdentifiersNotStrings(t *testing.T) {
	files := []gatescan.File{parse(t, "a.go", "package a\n\nvar _ = \"InvokeOnBestNode\"\n")}

	got := CheckRemovedSymbols(files, Exemption{})
	if len(got) != 0 {
		t.Fatalf("string literal is not an identifier, got %v", got)
	}
	if RemovedSymbols[0] != "InvokeOnBestNode" {
		t.Fatalf("removed-symbol table lost its first entry: %q", RemovedSymbols[0])
	}
}

func TestCheckRemovedSymbolsHonoursExemptions(t *testing.T) {
	files := []gatescan.File{
		parse(t, "capability/ports/state.go", "package ports\n\ntype State interface{ GetWorkingValue() }\n"),
		parse(t, "capability/registry/edit_record.go", "package registry\n\nvar _ = env.GetWorkingValue\n"),
		parse(t, "capability/registry/other.go", "package registry\n\nvar _ = env.GetWorkingValue\n"),
		parse(t, archToolingDir+"/symcheck.go", "package arch\n\nvar _ = RemovedSymbols\n"),
	}

	exempt := Exemption{
		Prefixes: []string{archToolingDir},
		Files: []string{
			"capability/ports/state.go",
			"capability/registry/edit_record.go",
		},
	}
	got := CheckRemovedSymbols(files, exempt)
	if len(got) != 1 {
		t.Fatalf("want only the unexempt file flagged, got %v", got)
	}
	if !strings.Contains(got[0], "capability/registry/other.go") {
		t.Errorf("wrong file flagged: %s", got[0])
	}
}
