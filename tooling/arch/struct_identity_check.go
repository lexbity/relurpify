package arch

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CheckStructIdentityConverters flags functions whose signature converts between
// two types that share the SAME simple name but come from DIFFERENT packages —
// e.g. `func toRuntimeSafetySpec(*agentspec.RuntimeSafetySpec) *RuntimeSafetySpec`.
//
// Such a converter is the reliable signature of a type that was FORKED across a
// package boundary: two structurally-identical definitions plus a translation
// layer between them. It is the worst form of duplication (you maintain two
// defs AND a bridge) and exactly the "no shims / no compat / no aliases" the
// charter forbids. The fix is to collapse the type to a single owner, not to
// bridge it.
//
// This is a HEURISTIC tripwire: it flags same-name-different-package converter
// signatures as *candidates*. A genuine representation converter between two
// distinct types that happen to share a simple name (e.g. a domain↔wire/proto
// boundary) is a legitimate exception — allowlist it under category "converter".
func CheckStructIdentityConverters(pkgs []GoPackage, root string, allowlist Allowlist) []string {
	var violations []string
	fset := token.NewFileSet()

	for _, pkg := range pkgs {
		pkgDir := PackageDir(root, pkg.ImportPath)
		for _, fname := range pkg.GoFiles {
			if strings.HasSuffix(fname, "_test.go") {
				continue
			}
			src, err := os.ReadFile(filepath.Clean(filepath.Join(pkgDir, fname)))
			if err != nil {
				continue
			}
			parsed, err := parser.ParseFile(fset, fname, src, parser.SkipObjectResolution)
			if err != nil {
				continue
			}

			for _, decl := range parsed.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Type == nil {
					continue
				}
				params := convNamedTypes(fn.Type.Params)
				results := convNamedTypes(fn.Type.Results)
				for _, p := range params {
					for _, r := range results {
						// Same simple type name, different package qualifier =
						// a cross-package converter for a forked type.
						if p.name == r.name && p.qual != r.qual {
							v := fmt.Sprintf(
								"converter: %s.%s bridges %s→%s (same type name, different package — collapse the forked type to one owner)",
								pkg.ImportPath, fn.Name.Name, convQualName(p), convQualName(r),
							)
							if !allowlist.Contains("converter", v) {
								violations = append(violations, v)
							}
						}
					}
				}
			}
		}
	}

	sort.Strings(violations)
	return violations
}

// convTypeRef is a named type reference: a simple type name plus its package
// qualifier ("" for a type local to the package being analysed).
type convTypeRef struct {
	qual string
	name string
}

func convQualName(t convTypeRef) string {
	if t.qual == "" {
		return t.name
	}
	return t.qual + "." + t.name
}

// convNamedTypes returns the named type references appearing directly in a
// field list (function params or results), unwrapping pointers.
func convNamedTypes(fl *ast.FieldList) []convTypeRef {
	if fl == nil {
		return nil
	}
	var out []convTypeRef
	for _, field := range fl.List {
		if ref, ok := convTypeRefOf(field.Type); ok {
			out = append(out, ref)
		}
	}
	return out
}

// convTypeRefOf extracts a named-type reference from an expression, unwrapping a
// single pointer. Non-named types (maps, slices, funcs, interfaces) yield false.
func convTypeRefOf(expr ast.Expr) (convTypeRef, bool) {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return convTypeRefOf(e.X)
	case *ast.Ident:
		return convTypeRef{qual: "", name: e.Name}, true
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok {
			return convTypeRef{qual: x.Name, name: e.Sel.Name}, true
		}
	}
	return convTypeRef{}, false
}
