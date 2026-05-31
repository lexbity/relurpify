package main

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

type Finding struct {
	File    string
	Line    int
	Message string
}

func main() {
	workspace := "."
	if len(os.Args) > 1 {
		workspace = os.Args[1]
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: resolve workspace: %v\n", err)
		os.Exit(2)
	}
	findings := audit(absWorkspace)
	if len(findings) > 0 {
		for _, f := range findings {
			fmt.Printf("%s:%d: %s\n", f.File, f.Line, f.Message)
		}
		os.Exit(1)
	}
}

func audit(workspaceAbs string) []Finding {
	if _, err := os.Stat(filepath.Join(workspaceAbs, "framework", "cfgload")); err != nil {
		return nil
	}

	var findings []Finding
	fset := token.NewFileSet()

	filepath.WalkDir(workspaceAbs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(workspaceAbs, path)
		if err != nil {
			return nil
		}

		isEnvExempt := strings.HasPrefix(rel, "framework/cfgload/") ||
			strings.HasPrefix(rel, "framework/runtimeenv/") ||
			hasTestsuitePathComponent(rel)

		isConfigExempt := strings.HasPrefix(rel, "framework/cfgload/") ||
			hasTestsuitePathComponent(rel)

		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		file, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(src), "\n")

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			x, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if x.Name != "os" {
				return true
			}

			pos := fset.Position(sel.Pos())
			snippet := ""
			if pos.Line > 0 && pos.Line <= len(lines) {
				snippet = strings.TrimSpace(lines[pos.Line-1])
			}

			switch sel.Sel.Name {
			case "Getenv", "Environ", "LookupEnv", "Setenv":
				if !isEnvExempt {
					findings = append(findings, Finding{
						File:    rel,
						Line:    pos.Line,
						Message: fmt.Sprintf("direct environment boundary violation: %s", snippet),
					})
				}

			case "ReadFile", "ReadDir", "WriteFile", "OpenFile", "Create":
				if !isConfigExempt && hasConfigPathIndicator(call.Args) {
					findings = append(findings, Finding{
						File:    rel,
						Line:    pos.Line,
						Message: fmt.Sprintf("direct config path boundary violation: %s", snippet),
					})
				}
			}

			return true
		})

		return nil
	})

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})

	return findings
}

func hasConfigPathIndicator(args []ast.Expr) bool {
	stack := make([]ast.Expr, 0, len(args))
	stack = append(stack, args...)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch expr := n.(type) {
		case *ast.BasicLit:
			if expr.Kind == token.STRING {
				v := strings.ToLower(expr.Value)
				if strings.Contains(v, "relurpify_cfg") ||
					strings.Contains(v, ".yaml") ||
					strings.Contains(v, ".policy") ||
					strings.Contains(v, "manifest") ||
					strings.Contains(v, "/security/") {
					return true
				}
			}
		case *ast.CallExpr:
			for i := len(expr.Args) - 1; i >= 0; i-- {
				stack = append(stack, expr.Args[i])
			}
		case *ast.BinaryExpr:
			stack = append(stack, expr.X, expr.Y)
		}
	}
	return false
}

func hasTestsuitePathComponent(rel string) bool {
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts {
		if part == "testsuite" {
			return true
		}
	}
	return false
}
