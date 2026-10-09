package gatescan

import (
	"go/ast"
	"os"
	"path/filepath"
	"testing"
)

// userconfigDir is the one tree permitted to read the process environment.
const userconfigDir = "userconfig"

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil { //nolint:gosec // test fixture tree under t.TempDir
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil { //nolint:gosec // test fixture source under t.TempDir
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func paths(files []File) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

func TestWalkSkipsTestsCachesAndHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pkg/prod.go", "package pkg\n")
	writeFile(t, dir, "pkg/prod_test.go", "package pkg\n")
	writeFile(t, dir, ".hidden/hidden.go", "package hidden\n")
	writeFile(t, dir, "vendor/vendored.go", "package vendored\n")
	writeFile(t, dir, ".gomodcache/cached.go", "package cached\n")
	writeFile(t, dir, "dist/built.go", "package built\n")

	files, parseErrors, err := Walk(Options{Root: dir})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(parseErrors) != 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrors)
	}
	if got := paths(files); len(got) != 1 || got[0] != "pkg/prod.go" {
		t.Fatalf("want only pkg/prod.go, got %v", got)
	}
}

func TestWalkIncludeTestsAndSkipDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pkg/prod.go", "package pkg\n")
	writeFile(t, dir, "pkg/prod_test.go", "package pkg\n")
	writeFile(t, dir, "testdata/fixture.go", "package fixture\n")

	files, _, err := Walk(Options{Root: dir, IncludeTests: true})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if got := paths(files); len(got) != 3 {
		t.Fatalf("want 3 files with tests included, got %v", got)
	}

	files, _, err = Walk(Options{Root: dir, SkipDirs: []string{"testdata"}})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if got := paths(files); len(got) != 1 || got[0] != "pkg/prod.go" {
		t.Fatalf("want testdata skipped, got %v", got)
	}
}

func TestWalkReportsParseErrorsWithoutAborting(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "broken.go", "package broken\nfunc (\n")
	writeFile(t, dir, "fine.go", "package fine\n")

	files, parseErrors, err := Walk(Options{Root: dir})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if got := paths(files); len(got) != 1 || got[0] != "fine.go" {
		t.Fatalf("want fine.go parsed, got %v", got)
	}
	if len(parseErrors) != 1 {
		t.Fatalf("want 1 parse error, got %v", parseErrors)
	}
	if want := "broken.go"; parseErrors[0][:len(want)] != want {
		t.Fatalf("parse error should name broken.go, got %q", parseErrors[0])
	}
}

func TestWalkPropagatesWalkErrors(t *testing.T) {
	if _, _, err := Walk(Options{Root: filepath.Join(t.TempDir(), "does-not-exist")}); err == nil {
		t.Fatal("want walk error for missing root")
	}
}

func TestParseSource(t *testing.T) {
	f, err := ParseSource("pkg/a.go", "package pkg\n\n// Doc.\nfunc F() {}\n")
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	if f.Path != "pkg/a.go" {
		t.Fatalf("path = %q", f.Path)
	}
	if f.Fset == nil || f.AST == nil {
		t.Fatal("want fset and AST populated")
	}
	if got := Position(f, f.AST.Name); got != "pkg/a.go:1" {
		t.Fatalf("Position = %q", got)
	}
	if _, err := ParseSource("bad.go", "package bad\nfunc (\n"); err == nil {
		t.Fatal("want parse error for malformed source")
	}
}

func TestImportsOfResolvesPlainAliasedAndDotted(t *testing.T) {
	f, err := ParseSource("a.go", `package a

import (
	"fmt"
	o "os"
	. "strings"
)

var _ = fmt.Sprintf
var _ = o.Getenv
var _ = ToUpper
`)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	imports := ImportsOf(f.AST)

	if !imports.Resolves(ident(f, "o"), "os") {
		t.Error("aliased import o should resolve to os")
	}
	if !imports.Resolves(ident(f, "fmt"), "fmt") {
		t.Error("plain import fmt should resolve to fmt")
	}
	if imports.Resolves(ident(f, "fmt"), "os") {
		t.Error("fmt must not resolve to os")
	}
	if !imports.IsDotImported("strings") {
		t.Error("want strings dot-imported")
	}
	if imports.IsDotImported("os") {
		t.Error("os is aliased, not dot-imported")
	}
}

func TestLiteralValue(t *testing.T) {
	f, err := ParseSource("a.go", "package a\n\nvar (\n\ta = \"quoted\"\n\tb = `raw`\n\tc = 7\n)\n")
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	cases := map[string]string{
		"a": "quoted",
		"b": "raw",
		"c": "",
	}
	for name, want := range cases {
		if got := LiteralValue(literalFor(f, name)); got != want {
			t.Errorf("literal %s = %q, want %q", name, got, want)
		}
	}
	if got := LiteralValue(nil); got != "" {
		t.Errorf("LiteralValue(nil) = %q, want empty", got)
	}
}

func TestHasPathPrefixMatchesWholeComponents(t *testing.T) {
	cases := []struct {
		path     string
		prefixes []string
		want     bool
	}{
		{userconfigDir + "/config/a.go", []string{userconfigDir}, true},
		{"userconfigx/a.go", []string{userconfigDir}, false},
		{"a/b/" + userconfigDir + "/c.go", []string{userconfigDir}, false},
		{"tooling/arch/cmd/x/main.go", []string{"tooling/arch"}, true},
		{"tooling/archx/main.go", []string{"tooling/arch"}, false},
		{"pkg/a.go", nil, false},
	}
	for _, tc := range cases {
		if got := HasPathPrefix(tc.path, tc.prefixes...); got != tc.want {
			t.Errorf("HasPathPrefix(%q, %v) = %v, want %v", tc.path, tc.prefixes, got, tc.want)
		}
	}
}

func TestContainsDir(t *testing.T) {
	if !ContainsDir("a/vendor/b/c.go", "vendor") {
		t.Error("want vendor matched at depth")
	}
	if ContainsDir("a/vendored/b.go", "vendor") {
		t.Error("vendored must not match vendor")
	}
	if ContainsDir("pkg/a.go", "vendor", "testdata") {
		t.Error("want no match")
	}
}

// ident returns the identifier named name from f, or nil.
func ident(f File, name string) *ast.Ident {
	var found *ast.Ident
	ast.Inspect(f.AST, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name && found == nil {
			found = id
		}
		return true
	})
	return found
}

// literalFor returns the BasicLit assigned to the package-level var named name.
func literalFor(f File, name string) *ast.BasicLit {
	for _, decl := range f.AST.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok.String() != "var" {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) == 0 || vs.Names[0].Name != name {
				continue
			}
			if lit, ok := vs.Values[0].(*ast.BasicLit); ok {
				return lit
			}
		}
	}
	return nil
}
