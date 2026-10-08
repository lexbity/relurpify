package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// readTestFile reads a file produced by the tool under test.
func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // path is inside t.TempDir(), created by this test
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func writeProfile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	return path
}

func TestParseProfileAggregatesStatementsPerPackage(t *testing.T) {
	path := writeProfile(t, `mode: set
codeburg.org/lexbit/relurpify/pkg/a/a.go:10.20,12.10 3 1
codeburg.org/lexbit/relurpify/pkg/a/b.go:5.30,7.2 2 0
codeburg.org/lexbit/relurpify/pkg/b/c.go:1.1,3.2 4 4
`)

	packages, err := parseProfile(path)
	if err != nil {
		t.Fatalf("parseProfile: %v", err)
	}
	if len(packages) != 2 {
		t.Fatalf("got %d packages, want 2", len(packages))
	}

	a := packages[0]
	if a.packagePath != "codeburg.org/lexbit/relurpify/pkg/a" {
		t.Errorf("first package = %q", a.packagePath)
	}
	if a.totalStmt != 5 || a.coveredStmt != 3 {
		t.Errorf("pkg a = %d/%d stmts, want 3/5", a.coveredStmt, a.totalStmt)
	}
	if got := a.percent(); got != 60.0 {
		t.Errorf("pkg a percent = %v, want 60", got)
	}

	b := packages[1]
	if b.totalStmt != 4 || b.coveredStmt != 4 {
		t.Errorf("pkg b = %d/%d stmts, want 4/4", b.coveredStmt, b.totalStmt)
	}
}

func TestParseProfileSortsByPackagePath(t *testing.T) {
	path := writeProfile(t, `mode: set
codeburg.org/lexbit/relurpify/z/z.go:1.1,2.2 1 1
codeburg.org/lexbit/relurpify/a/a.go:1.1,2.2 1 1
`)

	packages, err := parseProfile(path)
	if err != nil {
		t.Fatalf("parseProfile: %v", err)
	}
	if !strings.HasSuffix(packages[0].packagePath, "/a") || !strings.HasSuffix(packages[1].packagePath, "/z") {
		t.Fatalf("packages not sorted: %v", packages)
	}
}

func TestParseProfileSkipsModeAndMalformedLines(t *testing.T) {
	path := writeProfile(t, `mode: count
mode: atomic
codeburg.org/lexbit/relurpify/pkg/a/a.go:1.1,2.2 1 1
garbage line without counts
codeburg.org/lexbit/relurpify/pkg/a/a.go 1 1
codeburg.org/lexbit/relurpify/pkg/b/b.go:1.1,2.2 notanumber 1

codeburg.org/lexbit/relurpify/pkg/c/c.go:1.1,2.2 2 0
`)

	packages, err := parseProfile(path)
	if err != nil {
		t.Fatalf("parseProfile: %v", err)
	}
	if len(packages) != 2 {
		t.Fatalf("got %d packages, want 2 (a and c): %+v", len(packages), packages)
	}
}

// A package whose blocks all report zero statements never enters the tally,
// which is how type-only and test-only packages stay excluded.
func TestParseProfileExcludesZeroStatementPackages(t *testing.T) {
	path := writeProfile(t, `mode: set
codeburg.org/lexbit/relurpify/pkg/real/real.go:1.1,2.2 0 0
codeburg.org/lexbit/relurpify/pkg/typed/typed.go:1.1,2.2 7 3
`)

	packages, err := parseProfile(path)
	if err != nil {
		t.Fatalf("parseProfile: %v", err)
	}
	if len(packages) != 1 {
		t.Fatalf("got %d packages, want 1", len(packages))
	}
	if packages[0].packagePath != "codeburg.org/lexbit/relurpify/pkg/typed" {
		t.Fatalf("kept wrong package: %q", packages[0].packagePath)
	}
}

func TestExtractPackagePath(t *testing.T) {
	tests := []struct {
		location string
		want     string
	}{
		{"codeburg.org/lexbit/relurpify/pkg/file.go:12.34,14.16", "codeburg.org/lexbit/relurpify/pkg"},
		{"codeburg.org/lexbit/relurpify/file.go:1.1,2.2", "codeburg.org/lexbit/relurpify"},
		{"codeburg.org/lexbit/relurpify/a/b/c/file.go:1.1,2.2", "codeburg.org/lexbit/relurpify/a/b/c"},
		{"nolocation", ""},
		{"only/file.go", ""},
	}
	for _, tt := range tests {
		if got := extractPackagePath(tt.location); got != tt.want {
			t.Errorf("extractPackagePath(%q) = %q, want %q", tt.location, got, tt.want)
		}
	}
}

func TestFloorPercentTruncatesForSlack(t *testing.T) {
	pc := packageCoverage{packagePath: "p", totalStmt: 1000, coveredStmt: 627}
	if got := pc.floorPercent(); got != 62 {
		t.Errorf("floorPercent = %d, want 62", got)
	}
	full := packageCoverage{packagePath: "p", totalStmt: 100, coveredStmt: 100}
	if got := full.floorPercent(); got != 100 {
		t.Errorf("floorPercent(full) = %d, want 100", got)
	}
}

func TestLoadOverridesMissingFileMeansNoExplicitFloors(t *testing.T) {
	overrides, err := loadOverrides(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("loadOverrides: %v", err)
	}
	if len(overrides) != 0 {
		t.Fatalf("got %d overrides, want 0", len(overrides))
	}
}

func TestLoadOverridesParsesEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	body := `packages:
  codeburg.org/lexbit/relurpify/pkg/a:
    floor: 55
    reason: partial unit coverage
  codeburg.org/lexbit/relurpify/pkg/b:
    floor: 0
    reason: no tests yet
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	overrides, err := loadOverrides(path)
	if err != nil {
		t.Fatalf("loadOverrides: %v", err)
	}
	if got := overrides["codeburg.org/lexbit/relurpify/pkg/a"]; got.Floor != 55 || got.Reason != "partial unit coverage" {
		t.Errorf("pkg a override = %+v", got)
	}
	if got := overrides["codeburg.org/lexbit/relurpify/pkg/b"]; got.Floor != 0 {
		t.Errorf("pkg b floor = %d, want 0", got.Floor)
	}
}

func TestBuildReportAppliesOverrideOrDefaultFloor(t *testing.T) {
	packages := []packageCoverage{
		{packagePath: "listed", totalStmt: 100, coveredStmt: 60},
		{packagePath: "unlisted", totalStmt: 100, coveredStmt: 80},
	}
	overrides := map[string]override{"listed": {Floor: 55, Reason: "partial"}}

	entries := buildReport(packages, overrides, 70)
	if entries[0].floor != 55 || !entries[0].fromFile {
		t.Errorf("listed entry = %+v, want floor 55 from file", entries[0])
	}
	if entries[1].floor != 70 || entries[1].fromFile {
		t.Errorf("unlisted entry = %+v, want default floor 70", entries[1])
	}
	if entries[0].status() != "PASS" {
		t.Errorf("60%% against floor 55 should pass")
	}
}

func TestRunCheckFailsBelowFloorAndPassesAtFloor(t *testing.T) {
	packages := []packageCoverage{
		{packagePath: "at-floor", totalStmt: 100, coveredStmt: 55},
		{packagePath: "below-floor", totalStmt: 100, coveredStmt: 54},
	}
	overrides := map[string]override{"at-floor": {Floor: 55, Reason: "r"}, "below-floor": {Floor: 55, Reason: "r"}}

	if failures := runCheck(packages, overrides, 70); failures != 1 {
		t.Fatalf("runCheck failures = %d, want 1", failures)
	}
}

func TestRunCheckAppliesDefaultFloorToUnlistedPackages(t *testing.T) {
	packages := []packageCoverage{
		{packagePath: "new-pkg", totalStmt: 100, coveredStmt: 69},
		{packagePath: "fine-pkg", totalStmt: 100, coveredStmt: 70},
	}

	if failures := runCheck(packages, nil, 70); failures != 1 {
		t.Fatalf("runCheck failures = %d, want 1 (69%% below the default floor)", failures)
	}
}

func TestStaleOverrideEntriesReportsEntriesWithoutCoverageData(t *testing.T) {
	packages := []packageCoverage{{packagePath: "measured", totalStmt: 10, coveredStmt: 5}}
	overrides := map[string]override{
		"measured": {Floor: 50, Reason: "r"},
		"removed":  {Floor: 50, Reason: "r"},
	}

	stale := staleOverrideEntries(packages, overrides)
	if len(stale) != 1 || stale[0] != "removed" {
		t.Fatalf("stale = %v, want [removed]", stale)
	}
	if got := staleOverrideEntries(packages, nil); len(got) != 0 {
		t.Fatalf("stale with no overrides = %v, want empty", got)
	}
}

func TestRunUpdateBaselineRecordsCurrentCoverageAndPreservesReasons(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	packages := []packageCoverage{
		{packagePath: "raising", totalStmt: 1000, coveredStmt: 620}, // 62%, floor 55 recorded
		{packagePath: "fresh", totalStmt: 100, coveredStmt: 16},     // new entry
		{packagePath: "empty", totalStmt: 100, coveredStmt: 0},      // new entry, 0%
		{packagePath: "graduated", totalStmt: 100, coveredStmt: 72}, // >= default floor
		{packagePath: "strict", totalStmt: 100, coveredStmt: 80},    // floor above default
	}
	existing := map[string]override{
		"raising":   {Floor: 55, Reason: "original reason"},
		"graduated": {Floor: 60, Reason: "about to graduate"},
		"strict":    {Floor: 80, Reason: "intentionally strict"},
	}

	if err := runUpdateBaseline(packages, existing, 70, path); err != nil {
		t.Fatalf("runUpdateBaseline: %v", err)
	}

	got, err := loadOverrides(path)
	if err != nil {
		t.Fatalf("loadOverrides: %v", err)
	}

	if ov := got["raising"]; ov.Floor != 62 || ov.Reason != "original reason" {
		t.Errorf("raising = %+v, want floor 62 with preserved reason", ov)
	}
	if ov := got["fresh"]; ov.Floor != 16 || ov.Reason != reasonPartial {
		t.Errorf("fresh = %+v, want floor 16 with the partial-coverage reason", ov)
	}
	if ov := got["empty"]; ov.Floor != 0 || ov.Reason != reasonUntested {
		t.Errorf("empty = %+v, want floor 0 with the untested reason", ov)
	}
	if _, ok := got["graduated"]; ok {
		t.Error("graduated package should have been dropped from the overrides file")
	}
	if ov := got["strict"]; ov.Floor != 80 || ov.Reason != "intentionally strict" {
		t.Errorf("strict = %+v, want floor 80 kept untouched", ov)
	}
}

func TestRunUpdateBaselineRefusesRegressions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	packages := []packageCoverage{
		{packagePath: "regressed", totalStmt: 100, coveredStmt: 50}, // floor 55 recorded
	}
	existing := map[string]override{"regressed": {Floor: 55, Reason: "was here"}}

	err := runUpdateBaseline(packages, existing, 70, path)
	if err == nil {
		t.Fatal("expected regression refusal")
	}
	if !strings.Contains(err.Error(), "regressed") {
		t.Errorf("error should name the package, got: %v", err)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("overrides file must be left untouched on regression")
	}
}

func TestRunUpdateBaselineIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	packages := []packageCoverage{
		{packagePath: "a", totalStmt: 100, coveredStmt: 60},
		{packagePath: "b", totalStmt: 100, coveredStmt: 0},
	}

	if err := runUpdateBaseline(packages, nil, 70, path); err != nil {
		t.Fatalf("first update: %v", err)
	}
	first := readTestFile(t, path)

	existing, err := loadOverrides(path)
	if err != nil {
		t.Fatalf("loadOverrides: %v", err)
	}
	if err := runUpdateBaseline(packages, existing, 70, path); err != nil {
		t.Fatalf("second update: %v", err)
	}
	second := readTestFile(t, path)
	if first != second {
		t.Errorf("regeneration is not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestWrittenOverridesRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	overrides := map[string]override{
		"codeburg.org/lexbit/relurpify/pkg/a": {Floor: 16, Reason: "no unit tests yet; floor ratchets up from 0 as tests land"},
		"codeburg.org/lexbit/relurpify/pkg/b": {Floor: 55, Reason: "partial unit coverage; floor recorded from current coverage"},
	}

	if err := writeOverrides(path, overrides, 70); err != nil {
		t.Fatalf("writeOverrides: %v", err)
	}
	loaded, err := loadOverrides(path)
	if err != nil {
		t.Fatalf("loadOverrides: %v", err)
	}
	for pkg, want := range overrides {
		if got := loaded[pkg]; got != want {
			t.Errorf("%s: got %+v, want %+v", pkg, got, want)
		}
	}

	body := readTestFile(t, path)
	if !strings.Contains(body, "ratchet") {
		t.Error("written file is missing the mechanism header comment")
	}
	var probe overridesFile
	if err := yaml.Unmarshal([]byte(body), &probe); err != nil {
		t.Errorf("written file is not valid YAML: %v", err)
	}
}
