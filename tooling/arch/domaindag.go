package arch

import (
	"fmt"
	"sort"
	"strings"
)

// DomainDAG encodes the allowed domain import directions from §2.1.
// For each importing domain, allowedImports[domain] lists the set of domains
// it is permitted to import from. Self-imports (same domain) are always allowed.
// Domains not present (testsuite, tooling) are unrestricted.
var DomainDAG = domainDAG()

func domainDAG() map[string]map[string]bool {
	d := map[string][]string{
		"app": {
			"named", "cognitionzoo", "ayenitd", "execution",
			"context", "capability", "governance",
			"model", "jobs", "telemetry", "userconfig",
			"platform", "testsuite", "tooling",
		},
		"named": {
			"cognitionzoo", "ayenitd", "execution",
			"context", "capability", "governance",
			"model", "jobs", "telemetry", "userconfig",
			"platform", "testsuite", "tooling",
		},
		"cognitionzoo": {
			"ayenitd", "execution",
			"context", "capability", "governance",
			"model", "jobs", "telemetry", "userconfig",
			"platform", "testsuite", "tooling",
		},
		"ayenitd": {
			"execution",
			"context", "capability", "governance",
			"model", "jobs", "telemetry", "userconfig",
			"platform", "testsuite", "tooling",
		},
		"execution": {
			"context", "capability", "governance",
			"model", "jobs", "telemetry", "userconfig",
			"platform", "testsuite", "tooling",
		},
		"context": {
			"capability", "governance",
			"model", "jobs", "telemetry", "userconfig",
			"platform", "testsuite", "tooling",
		},
		"capability": {
			"governance",
			"model", "jobs", "telemetry", "userconfig",
			"platform", "testsuite", "tooling",
		},
		"governance": {
			"model", "jobs", "telemetry", "userconfig",
			"platform", "testsuite", "tooling",
		},
		"model": {
			"platform", "testsuite", "tooling",
		},
		"jobs": {
			"platform", "testsuite", "tooling",
		},
		"telemetry": {
			"platform", "testsuite", "tooling",
		},
		"userconfig": {
			"platform", "testsuite", "tooling",
		},
		"platform": {
			"capability", "governance", "context", "model",
			"testsuite", "tooling",
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
			if seen[a+"↔"+b] || seen[b+"↔"+a] {
				continue
			}
			seen[a+"↔"+b] = true

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
