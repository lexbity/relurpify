package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadExceptionsBuildsDirectionMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exceptions.yaml")
	write(t, dir, "exceptions.yaml", `version: 1
direction_violations:
  - src_domain: governance
    dst_domain: capability
  - src_domain: governance
    dst_domain: model
`)

	got := loadExceptions(path)
	require.True(t, got["governance"]["capability"], "first exception direction must be registered")
	require.True(t, got["governance"]["model"], "second exception direction must be registered")
	require.False(t, got["capability"]["governance"], "reverse direction must not be implied")
	require.Len(t, got, 1, "directions must group under their source domain")
}

func TestLoadExceptionsReturnsNilForMissingFile(t *testing.T) {
	require.Nil(t, loadExceptions(filepath.Join(t.TempDir(), "absent.yaml")))
}

func TestMainPassesOnCleanModuleTree(t *testing.T) {
	root := writeModuleFixture(t)

	code, out := runMainChild(t, root, "warn", "all")
	if code != 0 {
		t.Fatalf("clean module tree must exit 0 in warn mode, got %d (output: %s)", code, out)
	}
	if !strings.Contains(out, "[PASS] All domain checks passed") {
		t.Fatalf("clean tree must print the pass line, got %q", out)
	}
	for _, report := range []string{
		"domain-direction", "domain-cycles", "no-bucket",
		"governance-no-orchestration", "principal-context-write", "context-ports",
	} {
		if !strings.Contains(out, "[PASS] "+report) {
			t.Fatalf("check-all must report %s, got %q", report, out)
		}
	}
}

func TestMainEnforceModeExitsViaExitCodeOnCleanTree(t *testing.T) {
	root := writeModuleFixture(t)

	code, out := runMainChild(t, root, "enforce", "all")
	if code != 0 {
		t.Fatalf("clean module tree must exit 0 in enforce mode, got %d (output: %s)", code, out)
	}
}

// TestMainFlagsViolationsOnDirtyTree runs check-all in warn mode over a
// fixture that breaks four rules at once: a governance package importing
// execution (direction + governance-orchestration), a userconfig package
// calling ContextWithPrincipal outside execution/agentlifecycle
// (principal-context-write), and a context/ports package importing a module
// package (context-ports). Warn mode must report every failure yet exit 0.
func TestMainFlagsViolationsOnDirtyTree(t *testing.T) {
	root := writeDirtyFixture(t)

	code, out := runMainChild(t, root, "warn", "all")
	if code != 0 {
		t.Fatalf("warn mode must exit 0 even with violations, got %d (output: %s)", code, out)
	}
	for _, want := range []string{
		"[FAIL] domain-direction",
		"[FAIL] governance-no-orchestration",
		"[FAIL] principal-context-write",
		"[FAIL] context-ports",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dirty tree must report %q, got %q", want, out)
		}
	}
	if strings.Contains(out, "All domain checks passed") {
		t.Fatalf("dirty tree must not print the pass line, got %q", out)
	}
}

func TestMainRunsSingleCheck(t *testing.T) {
	root := writeModuleFixture(t)

	code, out := runMainChild(t, root, "warn", "direction")
	if code != 0 {
		t.Fatalf("single check must exit 0 in warn mode, got %d (output: %s)", code, out)
	}
	if !strings.Contains(out, "domain-direction") {
		t.Fatalf("direction check must run, got %q", out)
	}
	if strings.Contains(out, "domain-cycles") {
		t.Fatalf("check=direction must not run other checks, got %q", out)
	}
}

func TestMainReportsListPackagesFailure(t *testing.T) {
	code, out := runMainChild(t, t.TempDir(), "warn", "all")
	if code != 1 {
		t.Fatalf("module-less root must exit 1, got %d (output: %s)", code, out)
	}
	if !strings.Contains(out, "error listing packages") {
		t.Fatalf("failure must report the list error, got %q", out)
	}
}

func TestMainReportsExceptionsParseFailure(t *testing.T) {
	root := writeModuleFixture(t)
	write(t, root, filepath.Join("tooling", "arch", "exceptions.yaml"), ":\n- [")

	code, out := runMainChild(t, root, "warn", "all")
	if code != 1 {
		t.Fatalf("malformed exceptions must exit 1, got %d (output: %s)", code, out)
	}
	if !strings.Contains(out, "error parsing exceptions") {
		t.Fatalf("failure must report the parse error, got %q", out)
	}
}

// TestMainReportsExceptionsReadFailure covers the unreadable (not-missing)
// exceptions path: a directory where the exceptions file is expected.
func TestMainReportsExceptionsReadFailure(t *testing.T) {
	root := writeModuleFixture(t)
	if err := os.MkdirAll(filepath.Join(root, "tooling", "arch", "exceptions.yaml"), 0755); err != nil { //nolint:gosec // test fixture tree under t.TempDir
		t.Fatalf("mkdir: %v", err)
	}

	code, out := runMainChild(t, root, "warn", "all")
	if code != 1 {
		t.Fatalf("unreadable exceptions must exit 1, got %d (output: %s)", code, out)
	}
	if !strings.Contains(out, "error reading exceptions") {
		t.Fatalf("failure must report the read error, got %q", out)
	}
}

// TestMainChildProcess is the child entry point for the re-exec tests above.
// main() resolves its scan root from the working directory, takes its mode
// and check selection from flags, and always terminates via os.Exit; the
// child process makes that exit code observable and, with -test.gocoverdir
// shared by the parent, its counters merge into the parent's -coverprofile.
// Mode and check arrive via environment because the test binary's own flag
// parse rejects main's flags before main() registers them.
func TestMainChildProcess(t *testing.T) {
	if os.Getenv("DOMAINCHECK_CHILD") != "1" {
		t.Skip("child-only test; run via runMainChild")
	}
	mode := os.Getenv("DOMAINCHECK_MODE")
	if mode == "" {
		mode = "warn"
	}
	check := os.Getenv("DOMAINCHECK_CHECK")
	if check == "" {
		check = "all"
	}
	os.Args = []string{"domaincheck", "-mode=" + mode, "-check=" + check}
	main()
}

func runMainChild(t *testing.T, dir, mode, check string) (int, string) {
	t.Helper()
	args := []string{"-test.run=TestMainChildProcess$"}
	if d := coverDir(); d != "" {
		args = append(args, "-test.gocoverdir="+d)
	}
	cmd := exec.Command(os.Args[0], args...) //nolint:gosec // re-executes the test binary itself
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"DOMAINCHECK_CHILD=1",
		"DOMAINCHECK_MODE="+mode,
		"DOMAINCHECK_CHECK="+check,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("child run failed: %v (output: %s)", err, out)
		}
		return exitErr.ExitCode(), string(out)
	}
	return 0, string(out)
}

func coverDir() string {
	if f := flag.Lookup("test.gocoverdir"); f != nil {
		return f.Value.String()
	}
	return ""
}

// writeModuleFixture creates a minimal module whose single main package sits
// in a canonical domain directory and imports only the standard library, so
// every domain check passes.
func writeModuleFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "go.mod", "module codeburg.org/lexbit/relurpify\n\ngo 1.26\n")
	write(t, dir, filepath.Join("userconfig", "fixturemain", "main.go"),
		"package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hello\") }\n")
	return dir
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil { //nolint:gosec // test fixture tree under t.TempDir
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil { //nolint:gosec // test fixture source under t.TempDir
		t.Fatalf("write: %v", err)
	}
}

// writeDirtyFixture builds a module with four deliberate violations spanning
// four of the six domain checks (no-bucket needs a cross-domain fan-in and
// domain-cycles need an import cycle, both impractical in a minimal tree).
func writeDirtyFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mod := "codeburg.org/lexbit/relurpify"
	write(t, dir, "go.mod", "module "+mod+"\n\ngo 1.26\n")
	write(t, dir, filepath.Join("userconfig", "fixturemain", "main.go"),
		"package main\n\nfunc main() {}\n")
	write(t, dir, filepath.Join("governance", "ports", "ports.go"),
		"package ports\n\nfunc ContextWithPrincipal(ctx any, p any) any { return p }\n")
	write(t, dir, filepath.Join("execution", "runner", "runner.go"),
		"package runner\n\nfunc Run() {}\n")
	write(t, dir, filepath.Join("userconfig", "fixturelib", "lib.go"),
		"package fixturelib\n\nimport \""+mod+"/governance/ports\"\n\nvar _ = ports.ContextWithPrincipal\n")
	write(t, dir, filepath.Join("governance", "policy", "policy.go"),
		"package policy\n\nimport \""+mod+"/execution/runner\"\n\nvar _ = runner.Run\n")
	write(t, dir, filepath.Join("context", "ports", "leaf.go"),
		"package ports\n\nimport \""+mod+"/userconfig/fixturelib\"\n\nvar _ = fixturelib.Loaded\n")
	write(t, dir, filepath.Join("userconfig", "fixturelib", "loaded.go"),
		"package fixturelib\n\nconst Loaded = true\n")
	return dir
}
