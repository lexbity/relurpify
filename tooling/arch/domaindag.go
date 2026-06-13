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

const (
	Ayenitd_domaindag = "ayenitd"
	Capability_domaindag = "capability"
	Cognitionzoo_domaindag = "cognitionzoo"
	Context_domaindag = "context"
	Execution_domaindag = "execution"
	Governance_domaindag = "governance"
	Jobs_domaindag = "jobs"
	Model_domaindag = "model"
	Platform_domaindag = "platform"
	Telemetry_domaindag = "telemetry"
	Testsuite_domaindag = "testsuite"
	Tooling_domaindag = "tooling"
	Userconfig_domaindag = "userconfig"
	C_2194 = "↔"
)


// DomainDAG encodes the allowed domain import directions from §2.1.
// For each importing domain, allowedImports[domain] lists the set of domains
// it is permitted to import from. Self-imports (same domain) are always allowed.
// Domains not present (testsuite, tooling) are unrestricted.
var DomainDAG = domainDAG()

func domainDAG() map[string]map[string]bool {
	d := map[string][]string{
		"app": {
			"named", Cognitionzoo_domaindag, Ayenitd_domaindag, Execution_domaindag,
			Context_domaindag, Capability_domaindag, Governance_domaindag,
			Model_domaindag, Jobs_domaindag, Telemetry_domaindag, Userconfig_domaindag,
			Platform_domaindag, Testsuite_domaindag, Tooling_domaindag,
		},
		"named": {
			Cognitionzoo_domaindag, Ayenitd_domaindag, Execution_domaindag,
			Context_domaindag, Capability_domaindag, Governance_domaindag,
			Model_domaindag, Jobs_domaindag, Telemetry_domaindag, Userconfig_domaindag,
			Platform_domaindag, Testsuite_domaindag, Tooling_domaindag,
		},
		Cognitionzoo_domaindag: {
			Ayenitd_domaindag, Execution_domaindag,
			Context_domaindag, Capability_domaindag, Governance_domaindag,
			Model_domaindag, Jobs_domaindag, Telemetry_domaindag, Userconfig_domaindag,
			Platform_domaindag, Testsuite_domaindag, Tooling_domaindag,
		},
		Ayenitd_domaindag: {
			Execution_domaindag,
			Context_domaindag, Capability_domaindag, Governance_domaindag,
			Model_domaindag, Jobs_domaindag, Telemetry_domaindag, Userconfig_domaindag,
			Platform_domaindag, Testsuite_domaindag, Tooling_domaindag,
		},
		Execution_domaindag: {
			Context_domaindag, Capability_domaindag, Governance_domaindag,
			Model_domaindag, Jobs_domaindag, Telemetry_domaindag, Userconfig_domaindag,
			Platform_domaindag, Testsuite_domaindag, Tooling_domaindag,
		},
		Context_domaindag: {
			Capability_domaindag, Governance_domaindag,
			Model_domaindag, Jobs_domaindag, Telemetry_domaindag, Userconfig_domaindag,
			Platform_domaindag, Testsuite_domaindag, Tooling_domaindag,
		},
		Capability_domaindag: {
			Governance_domaindag,
			Model_domaindag, Jobs_domaindag, Telemetry_domaindag, Userconfig_domaindag,
			Platform_domaindag, Testsuite_domaindag, Tooling_domaindag,
		},
		Governance_domaindag: {
			Model_domaindag, Jobs_domaindag, Telemetry_domaindag, Userconfig_domaindag,
			Platform_domaindag, Testsuite_domaindag, Tooling_domaindag,
		},
		Model_domaindag: {
			Platform_domaindag, Testsuite_domaindag, Tooling_domaindag,
		},
		Jobs_domaindag: {
			Platform_domaindag, Testsuite_domaindag, Tooling_domaindag,
		},
		Telemetry_domaindag: {
			Platform_domaindag, Testsuite_domaindag, Tooling_domaindag,
		},
		Userconfig_domaindag: {
			Platform_domaindag, Testsuite_domaindag, Tooling_domaindag,
		},
		Platform_domaindag: {
			Capability_domaindag, Governance_domaindag, Context_domaindag, Model_domaindag,
			Testsuite_domaindag, Tooling_domaindag,
		},
	}
	out := make(map[string]map[string]bool, len(d))
	for src, targets := range d {
		m := make(map[string]bool, len(targets)+1)
		m[src] = true
		for _, t := range targets {
			m[t] = true
		}
		out[src] = m
	}
	// testsuite and tooling are unrestricted — they can import any domain.
	return out
}

// AllowedDomainImport reports whether domain src is allowed to import domain dst.
func AllowedDomainImport(src, dst string) bool {
	if src == dst {
		return true
	}
	m, ok := DomainDAG[src]
	if !ok {
		return true
	}
	return m[dst]
}

// DirectionException is a domain-pair exception for the direction checker.
type DirectionException struct {
	SrcDomain string `yaml:"src_domain"`
	DstDomain string `yaml:"dst_domain"`
	Phase     string `yaml:"phase"`
}

// CheckDomainDirection inspects every package-level import edge and reports any
// that violate the domain DAG defined in DomainDAG. In mode "warn" known
// domain-pair exceptions are suppressed; mode "enforce" reports all violations.
func CheckDomainDirection(pkgs []GoPackage, forward map[string][]string, mode string, domainExceptions map[string]map[string]bool) []string {
	var violations []string
	pkgDomain := make(map[string]string)
	for _, pkg := range pkgs {
		pkgDomain[pkg.ImportPath] = PackageDomain(pkg.ImportPath)
	}

	seen := make(map[string]bool)

	for _, pkg := range pkgs {
		srcDomain := pkgDomain[pkg.ImportPath]
		if srcDomain == "" {
			continue
		}
		imports := forward[pkg.ImportPath]
		for _, imp := range imports {
			if IsStandardLib(imp) {
				continue
			}
			dstDomain := PackageDomain(imp)
			if dstDomain == "" || dstDomain == srcDomain {
				continue
			}
			if AllowedDomainImport(srcDomain, dstDomain) {
				continue
			}

			if mode == "warn" && domainExceptions[srcDomain][dstDomain] {
				continue
			}

			key := fmt.Sprintf("direction: %s (%s) imports %s (%s)", pkg.ImportPath, srcDomain, imp, dstDomain)
			if seen[key] {
				continue
			}
			seen[key] = true
			violations = append(violations, key)
		}
	}
	sort.Strings(violations)
	return violations
}

// DomainCycleReport aggregates package-level imports to the domain level and
// reports every pair of domains (A, B) where A imports B and B imports A.
// Each entry includes the import counts for both directions. The output is
// sorted lexicographically by the first domain in the pair.
func DomainCycleReport(pkgs []GoPackage, forward map[string][]string) []string {
	reverse := make(map[string]map[string]int)

	pkgDomain := make(map[string]string)
	for _, pkg := range pkgs {
		pkgDomain[pkg.ImportPath] = PackageDomain(pkg.ImportPath)
	}

	for _, pkg := range pkgs {
		srcDomain := pkgDomain[pkg.ImportPath]
		if srcDomain == "" {
			continue
		}
		imports := forward[pkg.ImportPath]
		for _, imp := range imports {
			if IsStandardLib(imp) {
				continue
			}
			dstDomain := PackageDomain(imp)
			if dstDomain == "" || dstDomain == srcDomain {
				continue
			}
			if reverse[dstDomain] == nil {
				reverse[dstDomain] = make(map[string]int)
			}
			reverse[dstDomain][srcDomain]++
		}
	}

	forwardDomain := make(map[string]map[string]int)
	for dst, srcs := range reverse {
		for src, count := range srcs {
			if forwardDomain[src] == nil {
				forwardDomain[src] = make(map[string]int)
			}
			forwardDomain[src][dst] += count
			if forwardDomain[dst] == nil {
				forwardDomain[dst] = make(map[string]int)
			}
		}
	}

	seen := make(map[string]bool)
	var cycles []string
	for a := range forwardDomain {
		for b := range forwardDomain[a] {
			if a >= b {
				continue
			}
			ab := forwardDomain[a][b]
			ba := forwardDomain[b][a]
			if ba == 0 {
				continue
			}
			if seen[a+C_2194+b] || seen[b+C_2194+a] {
				continue
			}
			seen[a+C_2194+b] = true

			majority := "—"
			if ab > ba {
				majority = a + "→" + b
			} else if ba > ab {
				majority = b + "→" + a
			}
			cycles = append(cycles, fmt.Sprintf("%s ↔ %s: %d→%d (%s)", a, b, ab, ba, majority))
		}
	}
	sort.Strings(cycles)
	return cycles
}

// CheckNoBucket reports any package that is imported by ≥3 different domains
// and is type-only (no exported functions). This guard detects nascent
// shared-bucket packages (contracts/, core/, foundation/) forming.
//
// NFR-7 exemption: a package that is the public vocabulary of a single owning
// domain (lives at <domain>/ or <domain>/classification, contains only
// types/consts, zero funcs with logic) is NOT a bucket — it is owned by its
// domain and is that domain's public API surface.
func CheckNoBucket(pkgs []GoPackage, reverse map[string][]string, root string) []string {
	pkgMap := make(map[string]GoPackage)
	for _, pkg := range pkgs {
		pkgMap[pkg.ImportPath] = pkg
	}

	var violations []string
	for importPath, importers := range reverse {
		if IsStandardLib(importPath) {
			continue
		}
		pkg, ok := pkgMap[importPath]
		if !ok {
			continue
		}
		if pkg.Name == "main" || pkg.OnlyTestGoFiles {
			continue
		}

		domainSet := make(map[string]bool)
		for _, importer := range importers {
			impPkg, ok := pkgMap[importer]
			if !ok || impPkg.OnlyTestGoFiles {
				continue
			}
			d := PackageDomain(importer)
			if d != "" {
				domainSet[d] = true
			}
		}
		if len(domainSet) < 3 {
			continue
		}

		isTO, err := isTypeOnlyPackage(pkg, root)
		if err != nil || !isTO {
			continue
		}

		// NFR-7: Exempt single-owner pure-vocabulary packages.
		if isDomainVocabPackage(pkg, root) {
			continue
		}

		domains := make([]string, 0, len(domainSet))
		for d := range domainSet {
			domains = append(domains, d)
		}
		sort.Strings(domains)
		violations = append(violations, fmt.Sprintf("no-bucket: %s imported by %d domains (%s)", importPath, len(domainSet), strings.Join(domains, ", ")))
	}
	sort.Strings(violations)
	return violations
}

// isDomainVocabPackage reports whether a package is the public vocabulary of a
// single owning domain. It must live at <domain>/ or <domain>/classification
// and be type-only (no exported functions with logic). Such packages are
// exempt from the no-bucket rule per NFR-7.
func isDomainVocabPackage(pkg GoPackage, root string) bool {
	rel := TrimModulePrefix(pkg.ImportPath)
	if rel == "" {
		return false
	}
	parts := strings.SplitN(rel, "/", 3)
	if len(parts) < 1 {
		return false
	}
	domain := parts[0]
	// Must be a recognized domain root
	if !isKnownDomain(domain) {
		return false
	}
	// Must be at <domain>/ or <domain>/classification
	if len(parts) == 1 {
		// <domain>/ — domain root
		return isTypeOnlyPackageForVocab(pkg, root)
	}
	if len(parts) >= 2 && parts[1] == "classification" {
		// <domain>/classification[/…]
		return isTypeOnlyPackageForVocab(pkg, root)
	}
	return false
}

// isKnownDomain reports whether a directory name is a recognized top-level domain.
func isKnownDomain(name string) bool {
	for _, d := range TopLevelDomains {
		if d == name {
			return true
		}
	}
	return false
}

// isTypeOnlyPackageForVocab is a non-error variant of isTypeOnlyPackage used
// for the vocabulary exemption. Returns true if the package at root is type-only
// (no exported functions other than init); false if files can't be parsed.
func isTypeOnlyPackageForVocab(pkg GoPackage, root string) bool {
	pkgDir := PackageDir(root, pkg.ImportPath)
	files, err := os.ReadDir(pkgDir)
	if err != nil {
		return false
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

		src, err := os.ReadFile(filepath.Clean(filepath.Join(pkgDir, f.Name())))
		if err != nil {
			return false
		}

		parsed, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
		if err != nil {
			return false
		}

		for _, decl := range parsed.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Name.IsExported() && d.Name.Name != "init" {
					return false
				}
			case *ast.GenDecl:
				if d.Tok != token.TYPE && d.Tok != token.CONST && d.Tok != token.VAR {
					return false
				}
			default:
				return false
			}
		}
	}
	return true
}
