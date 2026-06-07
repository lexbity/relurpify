package arch

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ModulePath is the Go module import path for this project.
const ModulePath = "codeburg.org/lexbit/relurpify"

// TopLevelDomains lists the canonical top-level domain directories.
// Packages outside these are in "framework/", "platform/", "testsuite/", etc.
var TopLevelDomains = []string{
	"agents",
	"app",
	"ayenitd",
	"context",
	"framework",
	"governance",
	"named",
	"platform",
	"testsuite",
	"tooling",
}

// GoPackage represents a single Go package from `go list -json`.
type GoPackage struct {
	Dir             string   `json:"Dir"`
	ImportPath      string   `json:"ImportPath"`
	Name            string   `json:"Name"`
	Imports         []string `json:"Imports"`
	TestImports     []string `json:"TestImports"`
	XTestImports    []string `json:"XTestImports"`
	ForTest         string   `json:"ForTest"`
	OnlyTestGoFiles bool     `json:"-"`
	GoFiles         []string `json:"GoFiles"`
	TestGoFiles     []string `json:"TestGoFiles"`
}

// PackageDomain returns the top-level domain directory for a given import path.
func PackageDomain(importPath string) string {
	if importPath == ModulePath {
		return ""
	}
	if !strings.HasPrefix(importPath, ModulePath+"/") {
		return ""
	}
	rest := strings.TrimPrefix(importPath, ModulePath+"/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	return parts[0]
}

// ListPackages runs `go list -json ./...` and returns all packages.
func ListPackages(root string) ([]GoPackage, error) {
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = root
	cmd.Stderr = os.Stderr
	stdout, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	var pkgs []GoPackage
	dec := json.NewDecoder(strings.NewReader(string(stdout)))
	for dec.More() {
		var pkg GoPackage
		if err := dec.Decode(&pkg); err != nil {
			return nil, fmt.Errorf("decode go list output: %w", err)
		}
		if pkg.ForTest != "" {
			continue
		}
		pkg.OnlyTestGoFiles = len(pkg.GoFiles) == 0 && len(pkg.TestGoFiles) > 0
		pkgs = append(pkgs, pkg)
	}
	return pkgs, nil
}

// ImportGraph builds the forward and reverse import graph for all packages.
func ImportGraph(pkgs []GoPackage) (forward map[string][]string, reverse map[string][]string) {
	forward = make(map[string][]string)
	reverse = make(map[string][]string)
	for _, pkg := range pkgs {
		imports := make([]string, 0, len(pkg.Imports))
		for _, imp := range pkg.Imports {
			if imp == "C" {
				continue
			}
			imports = append(imports, imp)
		}
		forward[pkg.ImportPath] = imports
		for _, imp := range imports {
			reverse[imp] = append(reverse[imp], pkg.ImportPath)
		}
	}
	return forward, reverse
}

// TrimModulePrefix returns the module-relative path for an import path.
func TrimModulePrefix(importPath string) string {
	if importPath == ModulePath {
		return ""
	}
	return strings.TrimPrefix(importPath, ModulePath+"/")
}

// IsStandardLib returns true if the import path is not within this module.
func IsStandardLib(importPath string) bool {
	return !strings.HasPrefix(importPath, ModulePath+"/") && importPath != ModulePath
}

// PackageDir returns the filesystem directory for a package's import path.
func PackageDir(root, importPath string) string {
	return filepath.Join(root, TrimModulePrefix(importPath))
}

// DomainPackages groups packages by their top-level domain.
func DomainPackages(pkgs []GoPackage) map[string][]GoPackage {
	groups := make(map[string][]GoPackage)
	for _, pkg := range pkgs {
		domain := PackageDomain(pkg.ImportPath)
		groups[domain] = append(groups[domain], pkg)
	}
	return groups
}

// Report formats a list of violation strings.
func Report(name string, violations []string) string {
	if len(violations) == 0 {
		return fmt.Sprintf("[PASS] %s: no violations\n", name)
	}
	sort.Strings(violations)
	var b strings.Builder
	fmt.Fprintf(&b, "[FAIL] %s: %d violation(s)\n", name, len(violations))
	for _, v := range violations {
		fmt.Fprintf(&b, "  %s\n", v)
	}
	return b.String()
}
