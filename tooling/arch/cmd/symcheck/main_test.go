package main

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/tooling/arch"
	"codeburg.org/lexbit/relurpify/tooling/arch/gatescan"
)

func TestRunFailsOnRemovedSymbol(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "pkg/a.go", "package pkg\n\nfunc f() { InvokeOnBestNode(nil) }\n")

	if code := run(dir); code == 0 {
		t.Fatal("want non-zero exit for a removed symbol")
	}
}

func TestRunPassesOnCleanTree(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "pkg/a.go", "package pkg\n\n// CompiledStepExtra is a different identifier.\ntype CompiledStepExtra struct{}\n")

	if code := run(dir); code != 0 {
		t.Fatalf("want zero exit when no removed symbol is present, got %d", code)
	}
}

func TestRunReportsWalkErrors(t *testing.T) {
	if code := run(filepath.Join(t.TempDir(), "missing")); code == 0 {
		t.Fatal("want non-zero exit when the root cannot be walked")
	}
}

// TestExemptionsAreLoadBearing walks the live tree and asserts that every
// recorded exemption is doing real work. The State-interface files are exempt
// because GetWorkingValue survives there as a live, different declaration;
// if a refactor ever removes that collision the exemption becomes decorative
// and this test says so.
func TestExemptionsAreLoadBearing(t *testing.T) {
	files, _, err := gatescan.Walk(gatescan.Options{Root: repoRoot(t)})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	if got := arch.CheckRemovedSymbols(files, exempt); len(got) != 0 {
		t.Fatalf("gate must pass on the live tree, got %d violations: %v", len(got), got)
	}

	for _, f := range exempt.Files {
		pruned := arch.Exemption{
			Prefixes: exempt.Prefixes,
			Files:    without(exempt.Files, f),
		}
		if got := arch.CheckRemovedSymbols(files, pruned); len(got) == 0 {
			t.Errorf("file exemption %s is decorative: dropping it changes nothing", f)
		}
	}

}

func without(items []string, drop string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item != drop {
			out = append(out, item)
		}
	}
	return out
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the working directory")
		}
		dir = parent
	}
}

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil { //nolint:gosec // test fixture tree under t.TempDir
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil { //nolint:gosec // test fixture source under t.TempDir
		t.Fatalf("write: %v", err)
	}
}
