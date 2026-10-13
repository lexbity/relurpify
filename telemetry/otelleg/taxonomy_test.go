package otelleg

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// declaredEventTypeValues parses the telemetry package source (the parent
// directory) and returns the string value of every declared Event* constant.
//
// Deriving the taxonomy from source — rather than a checked-in copy — is what
// makes the severity and signal exhaustiveness tests airtight: a new event type
// cannot be added without failing them.
func declaredEventTypeValues(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("..")
	if err != nil {
		t.Fatalf("read telemetry package dir: %v", err)
	}
	var values []string
	seen := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join("..", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, ident := range valueSpec.Names {
					if !strings.HasPrefix(ident.Name, "Event") || len(valueSpec.Values) <= i {
						continue
					}
					lit, ok := valueSpec.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					value, err := strconv.Unquote(lit.Value)
					if err != nil || seen[value] {
						continue
					}
					seen[value] = true
					values = append(values, value)
				}
			}
		}
	}
	return values
}
