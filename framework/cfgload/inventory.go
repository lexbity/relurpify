package cfgload

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

// FindingKind classifies ambient configuration behavior.
type FindingKind string

const (
	FindingKindEnvLookup     FindingKind = "env_lookup"
	FindingKindConfigRead    FindingKind = "config_read"
	FindingKindPathResolve   FindingKind = "path_resolution"
	FindingKindRuntimeAccess FindingKind = "runtime_access"
)

// Finding records one ambient configuration behavior in a source file.
type Finding struct {
	Kind    FindingKind
	File    string
	Line    int
	Package string
	Symbol  string
	Snippet string
}

// Inventory is the reproducible output of the phase 1 scan.
type Inventory struct {
	Root     string
	Files    []string
	Findings []Finding
}

// PhaseOneFiles lists the files covered by the phase 1 inventory snapshot.
// The list mirrors the cleanup set in the phase plan, with the local sandbox
// runner path corrected to the actual repository layout.
func PhaseOneFiles() []string {
	return []string{
		"framework/agentenv/workspace.go",
		"framework/agentenv/composition.go",
		"framework/core/config.go",
		"framework/manifest/manifest.go",
		"framework/manifest/skill_manifest.go",
		"framework/skills/resolve.go",
		"framework/skills/policies.go",
		"platform/llm/config.go",
		"framework/sandbox/local_command_runner.go",
		"framework/templates/resolver.go",
		"app/dev-agent-cli/start.go",
		"app/dev-agent-cli/agents.go",
		"app/relurpish/runtime/config.go",
		"app/relurpish/runtime/runtime.go",
		"app/relurpish/tui/pane_aiprovider.go",
		"app/relurpish/tui/editor_supervisor.go",
		"app/relurpish/euclotui/pane_library.go",
		"app/relurpish/euclotui/pane_diff.go",
		"framework/manifest/contract_spec.go",
	}
}

var ambientSelectors = map[string]FindingKind{
	"os.Getenv":      FindingKindEnvLookup,
	"os.ReadFile":    FindingKindConfigRead,
	"os.ReadDir":     FindingKindConfigRead,
	"os.OpenFile":    FindingKindRuntimeAccess,
	"os.Getwd":       FindingKindPathResolve,
	"os.UserHomeDir": FindingKindPathResolve,
	"filepath.Abs":   FindingKindPathResolve,
}

// CollectPhaseOneInventory scans the phase 1 file set beneath root.
func CollectPhaseOneInventory(root string) (Inventory, error) {
	return CollectInventory(root, PhaseOneFiles())
}

// CollectInventory scans the provided relative file set beneath root.
func CollectInventory(root string, relFiles []string) (Inventory, error) {
	if strings.TrimSpace(root) == "" {
		return Inventory{}, &ScanError{Err: fmt.Errorf("root required")}
	}
	inv := Inventory{
		Root:  filepath.Clean(root),
		Files: append([]string(nil), relFiles...),
	}
	fset := token.NewFileSet()
	for _, rel := range inv.Files {
		path := filepath.Join(inv.Root, rel)
		findings, err := collectFindingsForFile(fset, path, rel)
		if err != nil {
			return Inventory{}, err
		}
		inv.Findings = append(inv.Findings, findings...)
	}
	sort.Slice(inv.Findings, func(i, j int) bool {
		a := inv.Findings[i]
		b := inv.Findings[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Symbol != b.Symbol {
			return a.Symbol < b.Symbol
		}
		return a.Kind < b.Kind
	})
	return inv, nil
}

func collectFindingsForFile(fset *token.FileSet, absPath, relPath string) ([]Finding, error) {
	src, err := os.ReadFile(absPath)
	if err != nil {
		return nil, &ScanError{Path: relPath, Err: err}
	}
	file, err := parser.ParseFile(fset, absPath, src, parser.ParseComments)
	if err != nil {
		return nil, &ScanError{Path: relPath, Err: err}
	}
	lines := strings.Split(string(src), "\n")
	var findings []Finding
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
		key := x.Name + "." + sel.Sel.Name
		kind, ok := ambientSelectors[key]
		if !ok {
			return true
		}
		pos := fset.Position(sel.Pos())
		snippet := ""
		if pos.Line > 0 && pos.Line <= len(lines) {
			snippet = strings.TrimSpace(lines[pos.Line-1])
		}
		findings = append(findings, Finding{
			Kind:    kind,
			File:    relPath,
			Line:    pos.Line,
			Package: file.Name.Name,
			Symbol:  key,
			Snippet: snippet,
		})
		return true
	})
	return findings, nil
}

// Filter returns findings that are outside the cfgload package.
func (i Inventory) Filter() []Finding {
	if len(i.Findings) == 0 {
		return nil
	}
	out := make([]Finding, 0, len(i.Findings))
	for _, finding := range i.Findings {
		if !strings.HasPrefix(finding.File, "framework/cfgload/") {
			out = append(out, finding)
		}
	}
	return out
}

// AuditStrict returns an error if any finding lives outside framework/cfgload.
func (i Inventory) AuditStrict() error {
	findings := i.Filter()
	if len(findings) == 0 {
		return nil
	}
	return &AuditError{Findings: findings}
}
