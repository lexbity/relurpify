// coverage parses a Go coverage profile and enforces a per-package
// minimum coverage threshold. It exits non-zero if any package falls
// below the threshold.
//
// Usage:
//
//	coverage -min=70 -profile coverage.out
//
// The profile format is the standard Go cover tool output:
//
//	mode: set
//	codeburg.org/lexbit/relurpify/pkg/file.go:line.col,line.col numStmt count
//
// Lines starting with "mode:" are skipped. Packages with zero statements
// are excluded from enforcement (e.g., main packages with only init()).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

type packageCoverage struct {
	packagePath string
	totalStmt   int
	coveredStmt int
}

func main() {
	minCoverage := flag.Float64("min", 70.0, "minimum coverage percentage")
	profilePath := flag.String("profile", "coverage.out", "path to coverage profile")
	flag.Parse()

	if *profilePath == "" {
		fmt.Fprintln(os.Stderr, "[FAIL] coverage: profile path is required")
		os.Exit(1)
	}

	packages, err := parseProfile(*profilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] coverage: %v\n", err)
		os.Exit(1)
	}

	failures := printReport(packages, *minCoverage)
	if failures > 0 {
		fmt.Printf("\n[FAIL] coverage: %d package(s) below %.0f%%\n", failures, *minCoverage)
		os.Exit(1)
	}
	fmt.Printf("\n[PASS] coverage: all packages at or above %.0f%%\n", *minCoverage)
}

func parseProfile(path string) ([]*packageCoverage, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	pkgMap := make(map[string]*packageCoverage)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "mode:") {
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}

		location := parts[0]
		fields := strings.Fields(parts[1])
		if len(fields) < 2 {
			continue
		}

		numStmt, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		count, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}

		pkgPath := extractPackagePath(location)
		if pkgPath == "" {
			continue
		}

		pc, ok := pkgMap[pkgPath]
		if !ok {
			pc = &packageCoverage{packagePath: pkgPath}
			pkgMap[pkgPath] = pc
		}
		pc.totalStmt += numStmt
		if count > 0 {
			pc.coveredStmt += numStmt
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	result := make([]*packageCoverage, 0, len(pkgMap))
	for _, pc := range pkgMap {
		if pc.totalStmt > 0 {
			result = append(result, pc)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].packagePath < result[j].packagePath
	})
	return result, nil
}

func printReport(packages []*packageCoverage, min float64) int {
	failures := 0
	fmt.Printf("[check] coverage floor (min %.0f%%):\n", min)
	for _, pc := range packages {
		pct := float64(pc.coveredStmt) / float64(pc.totalStmt) * 100.0
		status := "PASS"
		if pct < min {
			status = "FAIL"
			failures++
		}
		fmt.Printf("  [%s] %-60s %6.2f%% (%d/%d stmts)\n",
			status, pc.packagePath, pct, pc.coveredStmt, pc.totalStmt)
	}
	return failures
}

func extractPackagePath(location string) string {
	lastColon := strings.LastIndex(location, ":")
	if lastColon < 0 {
		return ""
	}
	filePath := location[:lastColon]
	lastSlash := strings.LastIndex(filePath, "/")
	if lastSlash < 0 {
		return ""
	}
	return filePath[:lastSlash]
}
