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

// CheckBuckets detects packages imported by > N domains that export only types.
// A "bucket" is a universal import target with no domain question of its own.
//
// threshold: maximum number of importing domains before a package is flagged.
func CheckBuckets(pkgs []GoPackage, reverse map[string][]string, threshold int, root string, allowlist Allowlist) []string {
	if threshold <= 0 {
		threshold = 3
	}

	var violations []string
	pkgMap := make(map[string]GoPackage)
	for _, pkg := range pkgs {
		pkgMap[pkg.ImportPath] = pkg
	}

	for importPath, importers := range reverse {
		if IsStandardLib(importPath) {
			continue
		}
		pkg, ok := pkgMap[importPath]
		if !ok {
			continue
		}
		if pkg.OnlyTestGoFiles {
			continue
		}
		if pkg.Name == "main" {
			continue
		}

		// Group importers by domain
		domainSet := make(map[string]bool)
		totalImporters := 0
		for _, importer := range importers {
			importerPkg, ok := pkgMap[importer]
			if !ok {
				continue
			}
			if importerPkg.OnlyTestGoFiles {
				continue
			}
			domain := PackageDomain(importer)
			if domain != "" {
				domainSet[domain] = true
			}
			totalImporters++
		}

		domainCount := len(domainSet)
		if domainCount <= threshold {
			continue
		}
		if totalImporters == 0 {
			continue
		}

		isTypeOnly, err := isTypeOnlyPackage(pkg, root)
		if err != nil || !isTypeOnly {
			continue
		}

		// NFR-7: Exempt single-owner pure-vocabulary packages.
		if isDomainVocabPackage(pkg, root) {
			continue
		}

		domainList := make([]string, 0, len(domainSet))
		for d := range domainSet {
			domainList = append(domainList, d)
		}
		sort.Strings(domainList)
		violation := fmt.Sprintf("bucket: %s imported by %d domains (%s)", importPath, domainCount, strings.Join(domainList, ", "))
		if !allowlist.Contains("bucket", violation) {
			violations = append(violations, violation)
		}
	}
	return violations
}

// isTypeOnlyPackage checks whether a package contains only type declarations,
// constants, and variables — no exported functions or methods.
func isTypeOnlyPackage(pkg GoPackage, root string) (bool, error) {
	pkgDir := PackageDir(root, pkg.ImportPath)
	files, err := os.ReadDir(pkgDir)
	if err != nil {
		return false, err
	}

	fset := token.NewFileSet()
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if !strings.HasSuffix(f.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(f.Name(), "_test.go") {
			continue
		}

		src, err := os.ReadFile(filepath.Join(pkgDir, f.Name()))
		if err != nil {
			return false, err
		}

		parsed, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
		if err != nil {
			return false, err
		}

		for _, decl := range parsed.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Name.IsExported() && d.Name.Name != "init" {
					return false, nil
				}
			case *ast.GenDecl:
				// Only skip type, const, and var specs.
				// Import specs are not relevant.
				if d.Tok != token.TYPE && d.Tok != token.CONST && d.Tok != token.VAR {
					return false, nil
				}
			default:
				return false, nil
			}
		}
	}
	return true, nil
}
