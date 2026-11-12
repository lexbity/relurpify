package arch

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

// CheckRelurpishV2Source checks the production interaction boundary. It uses
// syntax and resolved import names, so renaming an os import cannot hide writes.
func CheckRelurpishV2Source(filename string, source []byte) ([]string, error) {
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, filename, source, 0)
	if err != nil {
		return nil, err
	}
	imports := make(map[string]string)
	for _, item := range file.Imports {
		path, err := strconv.Unquote(item.Path.Value)
		if err != nil {
			return nil, err
		}
		name := path[strings.LastIndex(path, "/")+1:]
		if item.Name != nil {
			name = item.Name.Name
		}
		if name == "." {
			return []string{fmt.Sprintf("%s: unqualified import", set.Position(item.Pos()))}, nil
		}
		imports[name] = path
	}
	var violations []string
	report := func(position token.Pos, reason string) {
		violations = append(violations, fmt.Sprintf("%s: %s", set.Position(position), reason))
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.TypeSpec:
			if value.Assign.IsValid() {
				report(value.Pos(), "type alias")
			}
		case *ast.FuncDecl:
			if strings.Contains(value.Name.Name, "ForTest") || value.Name.Name == "NewTestRootModel" {
				report(value.Pos(), "production test mutation API")
			}
		case *ast.SelectorExpr:
			if value.Sel.Name == "FieldByName" {
				report(value.Pos(), "reflective widget field access")
			}
			if strings.Contains(filename, "euclotui/") {
				if name, ok := value.X.(*ast.Ident); ok && imports[name.Name] == "os" {
					switch value.Sel.Name {
					case "WriteFile", "Create", "OpenFile", "Remove", "RemoveAll", "Rename":
						report(value.Pos(), "filesystem mutation in execution view")
					}
				}
			}
		case *ast.BasicLit:
			if value.Kind == token.STRING {
				text, err := strconv.Unquote(value.Value)
				if err == nil && strings.Contains(text, "not yet implemented as palette command") {
					report(value.Pos(), "unimplemented advertised command")
				}
			}
		}
		return true
	})
	return violations, nil
}

// MissingRelurpishV2Tests rejects a successful command that lists no tests.
func MissingRelurpishV2Tests(listing string) []string {
	found := make(map[string]bool)
	for _, name := range strings.Split(listing, "\n") {
		found[strings.TrimSpace(name)] = true
	}
	var missing []string
	for _, name := range []string{
		"TestV2PlainTextEverySurface", "TestV2DraftSurvivesNavigation",
		"TestV2ApprovalCannotStealTyping", "TestV2PasteDoesNotSubmit",
		"TestV2ViewIsPure", "TestV2DoctorWithoutRuntime",
		"TestV2TextBatchesAreNeverControlKeys",
	} {
		if !found[name] {
			missing = append(missing, name)
		}
	}
	return missing
}
