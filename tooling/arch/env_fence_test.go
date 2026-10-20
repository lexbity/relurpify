package arch

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// envFencePattern is the text the env fence polices: a direct read of the
// process environment through the os package.
var envFencePattern = regexp.MustCompile(`os\.(Getenv|LookupEnv|Environ)`) //nolint:gochecknoglobals // immutable fence vocabulary

// scanEnvFence returns "path:line:text" for every line of a .go file under
// root that matches envFencePattern. It walks the tree directly instead of
// shelling out to ripgrep, so the fence tests hold in CI images that do not
// ship rg. It mirrors the Makefile gate's scope: hidden trees (which includes
// .gomodcache and .gocache), vendor, dist, and _test.go files are skipped.
func scanEnvFence(root string) ([]string, error) {
	var hits []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || name == "dist" || name == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		for i, line := range strings.Split(string(data), "\n") {
			if envFencePattern.MatchString(line) {
				hits = append(hits, fmt.Sprintf("%s:%d:%s", rel, i+1, line))
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	return hits, nil
}

// envFenceViolations filters scanEnvFence output down to the lines the fence
// actually polices: everything outside userconfig (the configuration boundary)
// and tooling/arch (the tree that names the API it forbids).
func envFenceViolations(root string) ([]string, error) {
	hits, err := scanEnvFence(root)
	if err != nil {
		return nil, err
	}
	var violations []string
	for _, line := range hits {
		if strings.Contains(line, "userconfig/") || strings.Contains(line, "tooling/arch/") {
			continue
		}
		violations = append(violations, line)
	}
	return violations, nil
}

func TestEnvFencePassesOnCleanTree(t *testing.T) {
	violations, err := envFenceViolations(repoRoot(t))
	if err != nil {
		t.Fatalf("scan repo tree: %v", err)
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

	hits, err := scanEnvFence(tmpDir)
	if err != nil {
		t.Fatalf("scan temp dir: %v", err)
	}

	found := false
	for _, line := range hits {
		if strings.Contains(line, "violation.go") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected fence to catch env access in violation.go, output: %v", hits)
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
