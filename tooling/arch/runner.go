package arch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Result holds the output of a single gate check.
type Result struct {
	Name       string
	Violations []string
}

// RunAll runs all architecture gates and returns their results.
func RunAll(root, allowlistPath string) ([]Result, error) {
	pkgs, err := ListPackages(root)
	if err != nil {
		return nil, fmt.Errorf("list packages: %w", err)
	}

	forward, reverse := ImportGraph(pkgs)

	allowlist, err := LoadAllowlist(allowlistPath)
	if err != nil {
		return nil, fmt.Errorf("load allowlist: %w", err)
	}

	// Collect raw violations (before allowlist filtering)
	rawViolations := map[string][]string{
		"cycle":     CheckCycles(forward, Allowlist{}),
		"layer":     CheckLayerDirection(pkgs, forward, Allowlist{}),
		"bucket":    CheckBuckets(pkgs, reverse, 3, root, Allowlist{}),
		"consumer":  CheckConsumers(pkgs, reverse, Allowlist{}),
		"forbidden": CheckForbiddenImports(pkgs, Allowlist{}),
		"converter": CheckStructIdentityConverters(pkgs, root, Allowlist{}),
		"sqlite":    CheckSQLiteFree(pkgs, Allowlist{}),
	}

	globRaw, _ := runGlobGate(root)
	stubRaw, _ := runStubGate(root)
	rawViolations["glob"] = globRaw
	rawViolations["stub"] = stubRaw

	// Filter against allowlist
	var results []Result
	for name, raw := range rawViolations {
		filtered := make([]string, 0, len(raw))
		for _, v := range raw {
			if !allowlist.Contains(name, v) {
				filtered = append(filtered, v)
			}
		}
		results = append(results, Result{Name: name, Violations: filtered})
	}

	// Validate allowlist entries against raw violations
	stale := ValidateAllowlist(allowlist, rawViolations)
	if len(stale) > 0 {
		results = append(results, Result{Name: "stale-allowlist", Violations: stale})
	}

	return results, nil
}

// ExitCode returns 0 if all results have no violations, 1 otherwise.
func ExitCode(results []Result) int {
	for _, r := range results {
		if len(r.Violations) > 0 {
			return 1
		}
	}
	return 0
}

// PrintResults prints all results to stdout.
func PrintResults(results []Result) {
	hadFailure := false
	for _, r := range results {
		if len(r.Violations) > 0 {
			hadFailure = true
		}
		fmt.Print(Report(r.Name, r.Violations))
	}
	if !hadFailure {
		fmt.Println("[PASS] All architecture gates passed")
	}
}

// runGlobGate searches for glob patterns in non-test Go files.
func runGlobGate(root string) ([]string, error) {
	var violations []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".gocache" || name == ".gomodcache" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isNonTestGoFile(path) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if strings.HasPrefix(rel, "tooling/arch") {
			return nil
		}
		data, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}
		if containsGlobPattern(string(data)) {
			relPath := rel
			if relPath == "" {
				relPath = path
			}
			violations = append(violations, fmt.Sprintf("glob: %s contains glob pattern", relPath))
		}
		return nil
	})
	return violations, err
}

// runStubGate searches for stub markers in non-test Go files.
func runStubGate(root string) ([]string, error) {
	var violations []string
	stubPatterns := []string{
		"for now",
		"placeholder",
		"would search",
		"TODO",
		"FIXME",
		"HACK",
		"XXX",
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".gocache" || name == ".gomodcache" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isNonTestGoFile(path) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if strings.HasPrefix(rel, "tooling/arch") {
			return nil
		}
		data, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}
		content := string(data)
		for _, pattern := range stubPatterns {
			if strings.Contains(content, pattern) {
				relPath := rel
				if relPath == "" {
					relPath = path
				}
				violations = append(violations, fmt.Sprintf("stub: %s contains %q", relPath, pattern))
				break
			}
		}
		return nil
	})
	return violations, err
}

func isNonTestGoFile(path string) bool {
	if !strings.HasSuffix(path, ".go") {
		return false
	}
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	return true
}

func containsGlobPattern(s string) bool {
	patterns := []string{
		"glob.Glob",
		"filepath.Glob",
		"glob.Match",
		"glob.Compile",
	}
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}
