package arch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvFencePassesOnCleanTree(t *testing.T) {
	repoRoot := repoRoot(t)
	cmd := exec.Command("rg", "-n", `os\.(Getenv|LookupEnv|Environ)`,
		"--glob", "*.go",
		"--glob", "!**/*_test.go",
		"--glob", "!.gomodcache/**",
		"--glob", "!.gocache/**",
		".")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		// rg exits 1 when no matches found — that's the pass case
		return
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var violations []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "userconfig/") {
			continue
		}
		violations = append(violations, line)
	}
	if len(violations) > 0 {
		t.Fatalf("env access outside userconfig detected:\n%s", strings.Join(violations, "\n"))
	}
}

func TestEnvFenceCatchesViolation(t *testing.T) {
	tmpDir := t.TempDir()
	violationPath := filepath.Join(tmpDir, "violation.go")
	if err := os.WriteFile(violationPath, []byte(`package main

import "os"

func main() {
	_ = os.Getenv("VIOLATION")
}
`), 0644); err != nil { //nolint:gosec // test writes a synthetic violation fixture under temp dir
		t.Fatal(err)
	}

	cmd := exec.Command("rg", "-n", `os\.(Getenv|LookupEnv|Environ)`, //nolint:gosec // test invokes rg on a temp dir to verify the fence catches a planted violation
		"--glob", "*.go",
		"--glob", "!**/*_test.go",
		tmpDir)
	out, _ := cmd.CombinedOutput()

	if !strings.Contains(string(out), "violation.go") {
		t.Fatalf("expected fence to catch env access in violation.go, output: %s", string(out))
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// From tooling/arch/, go up two levels to repo root
	return filepath.Clean(filepath.Join(cwd, "..", ".."))
}
