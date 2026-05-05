package euclo

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestBoundary_NoForbiddenImportsOrLegacyMarkers(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve boundary test path")
	}
	root := filepath.Dir(file)
	fset := token.NewFileSet()
	commentPattern := regexp.MustCompile(`(?m)^\s*//.*\b(TODO|FIXME|TBD)\b`)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		assertAllowedImports(t, path, file)

		if !strings.HasSuffix(path, "_test.go") {
			for _, group := range file.Comments {
				for _, comment := range group.List {
					if commentPattern.MatchString(comment.Text) {
						t.Fatalf("legacy marker found in %s: %s", path, strings.TrimSpace(comment.Text))
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk named/euclo: %v", err)
	}
}

func assertAllowedImports(t *testing.T, path string, file *ast.File) {
	t.Helper()
	for _, spec := range file.Imports {
		importPath := strings.Trim(spec.Path.Value, `"`)
		switch {
		case strings.Contains(importPath, "REFERENCE_ONLY/"):
			t.Fatalf("forbidden legacy import in %s: %s", path, importPath)
		case strings.Contains(importPath, "archaeo/"):
			t.Fatalf("forbidden archaeo import in %s: %s", path, importPath)
		case strings.Contains(importPath, "/platform/") && !strings.HasSuffix(importPath, "/platform/contracts"):
			t.Fatalf("forbidden platform import in %s: %s", path, importPath)
		}
	}
}
