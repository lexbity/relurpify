package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMainPassesOnCleanModuleTree runs main() in a child process whose working
// directory is a minimal clean module: all architecture gates pass, so the
// process must print the pass report and exit 0.
func TestMainPassesOnCleanModuleTree(t *testing.T) {
	root := writeModuleFixture(t)

	code, out := runMainChild(t, root)
	if code != 0 {
		t.Fatalf("clean module tree must exit 0, got %d (output: %s)", code, out)
	}
	if !strings.Contains(out, "[PASS] All architecture gates passed") {
		t.Fatalf("clean tree must print the pass report, got %q", out)
	}
}

// TestMainReportsListPackagesFailure runs main() in a child process rooted at
// a directory without a module, asserting the go list failure is reported on
// stderr and exits 1.
func TestMainReportsListPackagesFailure(t *testing.T) {
	nomod := t.TempDir()

	code, out := runMainChild(t, nomod)
	if code != 1 {
		t.Fatalf("module-less root must exit 1, got %d (output: %s)", code, out)
	}
	if !strings.Contains(out, "error: ") {
		t.Fatalf("failure must be reported on stderr, got %q", out)
	}
}

// TestMainChildProcess is the child entry point for the re-exec tests above.
// main() resolves its scan root from the working directory and always
// terminates via os.Exit; the child process makes that exit code observable
// and, with -test.gocoverdir shared by the parent, its counters merge into
// the parent's -coverprofile.
func TestMainChildProcess(t *testing.T) {
	if os.Getenv("ARCHCHECK_CHILD") != "1" {
		t.Skip("child-only test; run via runMainChild")
	}
	main()
}

func runMainChild(t *testing.T, dir string) (int, string) {
	t.Helper()
	args := []string{"-test.run=TestMainChildProcess$"}
	if d := coverDir(); d != "" {
		args = append(args, "-test.gocoverdir="+d)
	}
	cmd := exec.Command(os.Args[0], args...) //nolint:gosec // re-executes the test binary itself
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "ARCHCHECK_CHILD=1")
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
// every architecture gate passes. main() reads the allowlist from
// devdocs/ref/arch-allowlist.yaml under its working directory; a missing
// allowlist is an empty allowlist.
func writeModuleFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "go.mod", "module codeburg.org/lexbit/relurpify\n\ngo 1.26\n")
	write(t, dir, "userconfig/fixturemain/main.go",
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
