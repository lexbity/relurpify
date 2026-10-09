// Command coverage enforces a per-package test coverage floor.
//
// It reads a Go coverage profile (the output of
// `go test -coverprofile=coverage.out ./...`) and fails when any package
// falls below its floor. Two mechanisms combine, per Decision 5 of the build
// spec:
//
//  1. A default floor (-min, 70) applies to every package, so new code is
//     held to the full floor from birth.
//  2. Packages that have not reached the default floor are listed in an
//     explicit overrides file (tooling/coverage/overrides.yaml) with their
//     current coverage recorded as the floor plus a justification. That file
//     is the ratchet: the gate fails the moment a listed package drops below
//     its recorded floor, and entries are meant to be raised — or removed,
//     once the package reaches the default floor — as tests land. Nothing is
//     silent: every package below the default floor is visible in the file
//     with the reason it is still there.
//
// Packages with zero statements (type-only or interface-only packages, and
// packages with nothing but tests) produce no profile blocks and are
// excluded from enforcement.
//
// Modes:
//
//	coverage -profile coverage.out                    check every package against its floor
//	coverage -profile coverage.out -update-baseline   refresh overrides.yaml at current coverage
//
// -update-baseline never lowers a recorded floor: a package whose coverage
// dropped below its floor is reported as a regression and the file is left
// untouched, so the ratchet cannot be defeated by regenerating the baseline.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// packageCoverage is the per-package statement tally derived from the
// profile: statements are summed across every block of every file in the
// package, and a block counts as covered when its execution count is
// non-zero.
type packageCoverage struct {
	packagePath string
	totalStmt   int
	coveredStmt int
}

func (pc packageCoverage) percent() float64 {
	if pc.totalStmt == 0 {
		return 0
	}
	return float64(pc.coveredStmt) / float64(pc.totalStmt) * 100.0
}

// floorPercent is the recordable floor for the current coverage: the whole
// percentage at or below the measured value, leaving the fractional part as
// slack so ordinary statement churn cannot trip the ratchet.
func (pc packageCoverage) floorPercent() int {
	return int(math.Floor(pc.percent()))
}

// override is an explicit per-package floor recorded in the overrides file.
type override struct {
	Floor  int    `yaml:"floor"`
	Reason string `yaml:"reason"`
}

// overridesFile is the on-disk shape of the ratchet: default floors still
// come from -min, so the file only lists packages below the default.
type overridesFile struct {
	Packages map[string]override `yaml:"packages"`
}

// reportEntry is one rendered line of the coverage report.
type reportEntry struct {
	coverage packageCoverage
	floor    int
	fromFile bool
}

func (e reportEntry) status() string {
	if e.coverage.percent() < float64(e.floor) {
		return "FAIL"
	}
	return "PASS"
}

func main() {
	minCoverage := flag.Float64("min", 70.0, "default minimum coverage percentage for packages without an explicit floor")
	profilePath := flag.String("profile", "coverage.out", "path to coverage profile")
	overridesPath := flag.String("overrides", "tooling/coverage/overrides.yaml", "path to per-package floor overrides (the ratchet)")
	updateBaseline := flag.Bool("update-baseline", false, "rewrite the overrides file at current coverage instead of checking")
	flag.Parse()

	if *profilePath == "" {
		fmt.Fprintln(os.Stderr, "[FAIL] coverage: profile path is required")
		os.Exit(1)
	}
	if *minCoverage < 0 || *minCoverage > 100 {
		fmt.Fprintf(os.Stderr, "[FAIL] coverage: -min must be within 0..100, got %v\n", *minCoverage)
		os.Exit(1)
	}

	packages, err := parseProfile(*profilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] coverage: %v\n", err)
		os.Exit(1)
	}

	overrides, err := loadOverrides(*overridesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] coverage: %v\n", err)
		os.Exit(1)
	}

	if *updateBaseline {
		if err := runUpdateBaseline(packages, overrides, *minCoverage, *overridesPath); err != nil {
			fmt.Fprintf(os.Stderr, "[FAIL] coverage: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if failures := runCheck(packages, overrides, *minCoverage); failures > 0 {
		fmt.Printf("\n[FAIL] coverage: %d package(s) below floor (default floor %.0f%%)\n", failures, *minCoverage)
		os.Exit(1)
	}
	fmt.Printf("\n[PASS] coverage: all %d packages at or above their floors (default floor %.0f%%)\n",
		len(packages), *minCoverage)
}

// runCheck prints the per-package report and returns the number of packages
// below their floor.
func runCheck(packages []packageCoverage, overrides map[string]override, defaultFloor float64) int {
	failures := printReport(buildReport(packages, overrides, defaultFloor), defaultFloor)
	for _, stale := range staleOverrideEntries(packages, overrides) {
		fmt.Printf("  [warn] overrides entry for %s has no coverage data (package removed or zero statements?)\n", stale)
	}
	return failures
}

// buildReport pairs each measured package with the floor that applies to it.
func buildReport(packages []packageCoverage, overrides map[string]override, defaultFloor float64) []reportEntry {
	entries := make([]reportEntry, 0, len(packages))
	for _, pc := range packages {
		ov, listed := overrides[pc.packagePath]
		floor := int(defaultFloor)
		if listed {
			floor = ov.Floor
		}
		entries = append(entries, reportEntry{coverage: pc, floor: floor, fromFile: listed})
	}
	return entries
}

func printReport(entries []reportEntry, defaultFloor float64) int {
	failures := 0
	fmt.Printf("[check] coverage floor (default %.0f%%, %d package(s) with explicit floors):\n", defaultFloor, countListed(entries))
	for _, e := range entries {
		status := e.status()
		if status == "FAIL" {
			failures++
		}
		fmt.Printf("  [%s] %-58s %6.2f%% (floor %3d; %d/%d stmts)\n",
			status, e.coverage.packagePath, e.coverage.percent(),
			e.floor, e.coverage.coveredStmt, e.coverage.totalStmt)
	}
	return failures
}

func countListed(entries []reportEntry) int {
	n := 0
	for _, e := range entries {
		if e.fromFile {
			n++
		}
	}
	return n
}

// staleOverrideEntries lists overrides entries with no matching coverage
// data, which means the file references a package that no longer contributes
// statements.
func staleOverrideEntries(packages []packageCoverage, overrides map[string]override) []string {
	if len(overrides) == 0 {
		return nil
	}
	measured := make(map[string]bool, len(packages))
	for _, pc := range packages {
		measured[pc.packagePath] = true
	}
	var stale []string
	for pkg := range overrides {
		if !measured[pkg] {
			stale = append(stale, pkg)
		}
	}
	sort.Strings(stale)
	return stale
}

// runUpdateBaseline rewrites the overrides file at current coverage.
//
// Recorded floors never move down: a package below its recorded floor is a
// regression and aborts the rewrite with the file untouched, so the ratchet
// cannot be defeated by regenerating the baseline. Floors at or above the
// recorded floor advance to the current coverage, entries that graduated to
// the default floor are dropped, and packages newly below the default floor
// are added with a justification.
func runUpdateBaseline(packages []packageCoverage, existing map[string]override, defaultFloor float64, path string) error {
	var regressions []string
	updated := make(map[string]override, len(existing))
	var notes []string

	for _, pc := range packages {
		pct := pc.percent()
		currentFloor := pc.floorPercent()
		ov, listed := existing[pc.packagePath]

		switch {
		case listed && float64(ov.Floor) > pct:
			regressions = append(regressions, fmt.Sprintf(
				"%s: coverage %.2f%% is below recorded floor %d%%", pc.packagePath, pct, ov.Floor))
			continue
		case listed:
			// Floors only ratchet up, and a floor above the default is an
			// intentional tightening that is kept as-is.
			floor := currentFloor
			if ov.Floor > floor {
				floor = ov.Floor
			}
			if float64(floor) >= defaultFloor {
				if ov.Floor > int(defaultFloor) {
					notes = append(notes, fmt.Sprintf("kept strict floor: %s at %d%%", pc.packagePath, ov.Floor))
					updated[pc.packagePath] = ov
					continue
				}
				notes = append(notes, fmt.Sprintf("graduated: %s at %.2f%% (default floor %.0f%%)",
					pc.packagePath, pct, defaultFloor))
				continue
			}
			if floor != ov.Floor {
				notes = append(notes, fmt.Sprintf("raised floor: %s %d%% -> %d%%", pc.packagePath, ov.Floor, floor))
			}
			updated[pc.packagePath] = override{Floor: floor, Reason: ov.Reason}
		case pct < defaultFloor:
			reason := reasonPartial
			if currentFloor == 0 {
				reason = reasonUntested
			}
			updated[pc.packagePath] = override{Floor: currentFloor, Reason: reason}
			notes = append(notes, fmt.Sprintf("new entry: %s at %.2f%%", pc.packagePath, pct))
		}
	}

	if len(regressions) > 0 {
		sort.Strings(regressions)
		return fmt.Errorf("%d package(s) below their recorded floor; refusing to rewrite %s.\n"+
			"       restore the tests or edit the floor explicitly:\n  - %s",
			len(regressions), path, strings.Join(regressions, "\n  - "))
	}

	if err := writeOverrides(path, updated, defaultFloor); err != nil {
		return err
	}
	sort.Strings(notes)
	for _, n := range notes {
		fmt.Printf("  [info] %s\n", n)
	}
	fmt.Printf("[PASS] coverage: baseline updated: %d entr(y|ies) in %s\n", len(updated), path)
	return nil
}

const (
	reasonUntested = "no unit tests yet; floor ratchets up from 0 as tests land"
	reasonPartial  = "partial unit coverage; floor recorded from current coverage, raise as tests land"
)

// writeOverrides renders the overrides file: a header documenting the
// mechanism, then one entry per package below the default floor, sorted by
// package path.
func writeOverrides(path string, overrides map[string]override, defaultFloor float64) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Per-package coverage floors below the %.0f%% default.\n", defaultFloor)
	b.WriteString("#\n")
	b.WriteString("# This file is the ratchet: the coverage gate fails the moment a package\n")
	b.WriteString("# drops below its recorded floor. Raise an entry as coverage improves and\n")
	b.WriteString("# delete it once the package reaches the default floor. Floors may only be\n")
	b.WriteString("# raised or removed through this file, never lowered silently -- regenerating\n")
	b.WriteString("# the baseline (make coverage-baseline) refuses any package that regressed\n")
	b.WriteString("# below its floor, and packages not listed here are held to the default.\n")
	b.WriteString("packages:\n")

	paths := make([]string, 0, len(overrides))
	for pkg := range overrides {
		paths = append(paths, pkg)
	}
	sort.Strings(paths)

	for _, pkg := range paths {
		ov := overrides[pkg]
		encodedReason, err := yaml.Marshal(ov.Reason)
		if err != nil {
			return fmt.Errorf("encode reason for %s: %w", pkg, err)
		}
		fmt.Fprintf(&b, "  %s:\n", pkg)
		fmt.Fprintf(&b, "    floor: %d\n", ov.Floor)
		fmt.Fprintf(&b, "    reason: %s\n", strings.TrimRight(string(encodedReason), "\n"))
	}

	// 0600 keeps gosec's write-permission rule satisfied; git normalizes
	// non-executable file modes on commit, so the checked-in file is 0644.
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// loadOverrides reads the ratchet file. A missing file means no explicit
// floors, which makes the gate fall back to the default floor for every
// package; callers that need the file to exist pass it explicitly.
func loadOverrides(path string) (map[string]override, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the operator-supplied -overrides flag of a developer-run gate tool, not untrusted input
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]override{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var file overridesFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if file.Packages == nil {
		return map[string]override{}, nil
	}
	return file.Packages, nil
}

// parseProfile aggregates a Go coverage profile by package. Profile lines
// look like:
//
//	mode: set
//	codeburg.org/lexbit/relurpify/pkg/file.go:12.34,14.16 2 1
//
// Lines that do not parse (other than the mode header) are skipped, and
// packages whose blocks sum to zero statements are dropped.
func parseProfile(path string) ([]packageCoverage, error) {
	file, err := os.Open(path) //nolint:gosec // path is the operator-supplied -profile flag of a developer-run gate tool, not untrusted input
	if err != nil {
		return nil, fmt.Errorf("open profile: %w", err)
	}
	// Read-only handle on the operator-supplied profile; a close failure has
	// no effect on the already-scanned content.
	defer func() { _ = file.Close() }()

	pkgMap := make(map[string]*packageCoverage)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}

		location, counts, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		fields := strings.Fields(counts)
		if len(fields) < 2 {
			continue
		}
		numStmt, err := strconv.Atoi(fields[0])
		if err != nil || numStmt <= 0 {
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
		return nil, fmt.Errorf("scan profile: %w", err)
	}

	result := make([]packageCoverage, 0, len(pkgMap))
	for _, pc := range pkgMap {
		if pc.totalStmt > 0 {
			result = append(result, *pc)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].packagePath < result[j].packagePath
	})
	return result, nil
}

// extractPackagePath turns a profile location
// (module/pkg/file.go:line.col,line.col) into the package path. The line
// suffix is discarded at the last colon before the line/column pair.
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
