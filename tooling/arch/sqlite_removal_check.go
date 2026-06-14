package arch

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

const (
	Migration_sqlite_removal_check = "migration"
)

// SQLiteFreePackages lists module-relative package prefixes that must not
// import "database/sql" or "github.com/mattn/go-sqlite3" in production code.
// Migration files explicitly named "migration" are exempt.
var SQLiteFreePackages = []string{
	"context/knowledge/ast",
	"context/knowledge/graphdb",
	"context/knowledge/search",
	"context/knowledge/ingestion",
}

// CheckSQLiteFree reports packages that illegally import database/sql or
// go-sqlite3. Migration files (containing "migration" in the path/filename) are exempt.
func CheckSQLiteFree(pkgs []GoPackage, allowlist Allowlist) []string {
	var violations []string
	sqliteImports := []string{
		"database/sql",
		"github.com/mattn/go-sqlite3",
	}

	fset := token.NewFileSet()

	for _, pkg := range pkgs {
		if !hasAnyPrefix(pkg.ImportPath, SQLiteFreePackages) {
			continue
		}
		// Exempt package paths containing "migration".
		if hasPathPrefix(pkg.ImportPath, Migration_sqlite_removal_check) || containsInPath(pkg.ImportPath, Migration_sqlite_removal_check) {
			continue
		}

		// Check non-migration Go files in this package
		var goFiles []string
		for _, f := range pkg.GoFiles {
			if !strings.Contains(strings.ToLower(f), Migration_sqlite_removal_check) {
				goFiles = append(goFiles, f)
			}
		}
		for _, f := range pkg.TestGoFiles {
			if !strings.Contains(strings.ToLower(f), Migration_sqlite_removal_check) {
				goFiles = append(goFiles, f)
			}
		}

		for _, filename := range goFiles {
			fullPath := filepath.Join(pkg.Dir, filename)
			fileAst, err := parser.ParseFile(fset, fullPath, nil, parser.ImportsOnly)
			if err != nil {
				continue // skip files that fail to parse
			}
			for _, impSpec := range fileAst.Imports {
				if impSpec.Path == nil {
					continue
				}
				impPath := strings.Trim(impSpec.Path.Value, "\"")
				for _, forbid := range sqliteImports {
					if impPath == forbid {
						v := pkg.ImportPath + " imports " + forbid + " (SQLite banned in graph/AST packages)"
						if !allowlist.Contains("sqlite", v) {
							violations = append(violations, v)
						}
					}
				}
			}
		}
	}
	return violations
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if hasPathPrefix(s, p) {
			return true
		}
	}
	return false
}

func containsInPath(s, substr string) bool {
	for _, part := range splitPath(s) {
		if part == substr {
			return true
		}
	}
	return false
}

func splitPath(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
