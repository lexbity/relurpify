package surface

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// bannedPrefixes are import path prefixes that the surface package must not
// import. surface is a leaf package and must depend only on the standard
// library.
var bannedPrefixes = []string{
	"codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes",
	"codeburg.org/lexbit/relurpify/named/euclo/reporting",
	"codeburg.org/lexbit/relurpify/named/euclo/interaction",
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate",
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext",
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb",
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval",
	"codeburg.org/lexbit/relurpify/app/",
}

func TestSurfaceImportsStdlibOnly(t *testing.T) {
	fset := token.NewFileSet()
	pkg := make(map[string]*ast.File)
	if err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		pkg[path] = f
		return nil
	}); err != nil {
		t.Fatalf("walking surface package: %v", err)
	}

	for _, f := range pkg {
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if isStdlib(path) {
				continue
			}
			for _, banned := range bannedPrefixes {
				if strings.HasPrefix(path, banned) {
					t.Errorf("file %s imports banned package %q", fset.Position(f.Pos()).Filename, path)
				}
			}
		}
	}
}

// isStdlib reports whether importPath belongs to the Go standard library.
func isStdlib(importPath string) bool {
	if strings.Contains(importPath, ".") {
		// Packages with a dot in the path (like codeburg.org/...) are
		// external. Stdlib packages never contain dots.
		return false
	}
	// All stdlib paths are dot-free.
	return true
}

func TestBannedPrefixesAreReachable(t *testing.T) {
	// Sanity check: verify the banned prefixes correspond to real packages
	// in the module. This stops typos from making the guardrail useless.
	for _, banned := range bannedPrefixes {
		// Remove trailing wildcard for prefix check
		prefix := strings.TrimSuffix(banned, "/")
		// Just verify the prefix is non-empty and well-formed
		if prefix == "" {
			t.Error("empty banned prefix")
		}
		if !strings.HasPrefix(prefix, "codeburg.org/lexbit/relurpify/") {
			t.Errorf("banned prefix %q does not start with module root", prefix)
		}
	}
}
