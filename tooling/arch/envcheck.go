package arch

import (
	"fmt"
	"go/ast"
	"sort"

	"codeburg.org/lexbit/relurpify/tooling/arch/gatescan"
)

// EnvAccessFuncs are the os package functions that read the process
// environment. Configuration enters the program through userconfig/config
// only; every other read site must be justified or removed.
var EnvAccessFuncs = []string{"Getenv", "LookupEnv", "Environ"} //nolint:gochecknoglobals // immutable os-package vocabulary the gate matches on

// CheckEnvAccess reports any os.Getenv/os.LookupEnv/os.Environ reference in
// files outside allowedRoots.
//
// Resolution happens on the syntax tree, so the gate also catches what the
// grep version cannot: an aliased import (`import o "os"; o.Getenv(...)`), a
// dot import (`import . "os"; Getenv(...)`), and references that are passed
// around without being called (`environ := os.Environ`).
func CheckEnvAccess(files []gatescan.File, allowedRoots ...string) []string {
	envFuncs := map[string]bool{}
	for _, name := range EnvAccessFuncs {
		envFuncs[name] = true
	}

	var violations []string
	for _, f := range files {
		if gatescan.HasPathPrefix(f.Path, allowedRoots...) {
			continue
		}
		imports := gatescan.ImportsOf(f.AST)

		ast.Inspect(f.AST, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.SelectorExpr:
				ident, ok := node.X.(*ast.Ident)
				if !ok || !envFuncs[node.Sel.Name] {
					return true
				}
				if imports.Resolves(ident, "os") {
					violations = append(violations, fmt.Sprintf(
						"env: %s reads the process environment via os.%s", gatescan.Position(f, node), node.Sel.Name))
				}
			case *ast.Ident:
				// Bare identifier from a dot import of "os".
				if envFuncs[node.Name] && imports.IsDotImported("os") {
					violations = append(violations, fmt.Sprintf(
						"env: %s reads the process environment via dot-imported %s", gatescan.Position(f, node), node.Name))
				}
			}
			return true
		})
	}
	sort.Strings(violations)
	return violations
}
