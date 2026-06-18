package arch

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
)

// principalWriteFunc is the only function that writes the governance:principal
// context key. Detecting a call to it is what this check is about.
const principalWriteFunc = "ContextWithPrincipal"

// CheckPrincipalContextWrite reports any package outside execution/agentlifecycle
// whose production source actually calls ContextWithPrincipal.
//
// NFR-8: The Principal context key MUST be writable only from
// execution/agentlifecycle. Everywhere else reads via PrincipalFromContext.
//
// The check is call-based, not import-based: it parses each package's production
// .go files and looks for a qualified `<pkg>.ContextWithPrincipal` selector.
// Importing governance/ports for the read side (PrincipalFromContext) or for its
// view types is therefore NOT a violation. The defining package (governance/ports)
// refers to the function by its bare identifier internally and is never flagged.
func CheckPrincipalContextWrite(pkgs []GoPackage) []string {
	var violations []string
	fset := token.NewFileSet()

	for _, pkg := range pkgs {
		if pkg.OnlyTestGoFiles {
			continue
		}
		rel := TrimModulePrefix(pkg.ImportPath)
		// execution/agentlifecycle is the sole authorized writer.
		if rel == "execution/agentlifecycle" || strings.HasPrefix(rel, "execution/agentlifecycle/") {
			continue
		}

		for _, filename := range pkg.GoFiles {
			fullPath := filepath.Join(pkg.Dir, filename)
			fileAst, err := parser.ParseFile(fset, fullPath, nil, 0)
			if err != nil {
				continue // skip files that fail to parse
			}
			if fileCallsPrincipalWrite(fileAst) {
				violations = append(violations, fmt.Sprintf(
					"principal-context-write: %s calls %s outside execution/agentlifecycle (write side restricted by NFR-8)",
					pkg.ImportPath, principalWriteFunc,
				))
				break // one violation per package is enough
			}
		}
	}
	sort.Strings(violations)
	return violations
}

// fileCallsPrincipalWrite reports whether the file references a qualified
// selector of the form `<ident>.ContextWithPrincipal`. The function name is
// unique to governance/ports, so the selector name alone is a precise signal.
func fileCallsPrincipalWrite(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel != nil && sel.Sel.Name == principalWriteFunc {
			found = true
			return false
		}
		return true
	})
	return found
}
