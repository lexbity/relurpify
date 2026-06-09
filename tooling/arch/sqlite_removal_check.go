package arch

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
// go-sqlite3. Migration files (containing "migration" in the path) are exempt.
func CheckSQLiteFree(pkgs []GoPackage, allowlist Allowlist) []string {
	var violations []string
	sqliteImports := []string{
		"database/sql",
		"github.com/mattn/go-sqlite3",
	}

	for _, pkg := range pkgs {
		if !hasAnyPrefix(pkg.ImportPath, SQLiteFreePackages) {
			continue
		}
		// Exempt migration directories.
		if hasPathPrefix(pkg.ImportPath, "migration") || containsInPath(pkg.ImportPath, "migration") {
			continue
		}
		for _, forbid := range sqliteImports {
			for _, imp := range pkg.Imports {
				if imp == forbid {
					v := pkg.ImportPath + " imports " + forbid + " (SQLite banned in graph/AST packages)"
					if !allowlist.Contains("sqlite", v) {
						violations = append(violations, v)
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
