package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMainGeneratesConfigIntoOutputDir runs main() in-process on the success
// path (which returns without exiting) and asserts the generated tree lands
// in the requested directory.
func TestMainGeneratesConfigIntoOutputDir(t *testing.T) {
	outDir := t.TempDir()
	setArgs(t, []string{"generate-config", outDir})

	main()

	// The embedded "workspace" subtree maps onto the config root (its prefix
	// is stripped) and the "prompts" subtree keeps its prompts/ prefix.
	for _, rel := range []string{
		"workspace.yaml",
		filepath.Join("prompts", "framework", "base.system.prompt"),
		filepath.Join("tools", "shell", "bash.tool.yaml"),
		filepath.Join("model", "profiles", "default.llm.yaml"),
	} {
		if _, err := os.Stat(filepath.Join(outDir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("generated tree must contain %s: %v", rel, err)
		}
	}
}

// TestMainUsageExitsNonZero runs main() in a child process sharing the
// coverage directory, asserting the missing-argument usage error exits 1.
func TestMainUsageExitsNonZero(t *testing.T) {
	code, out := runMainChild(t, "usage", "")
	if code != 1 {
		t.Fatalf("usage error must exit 1, got %d (output: %s)", code, out)
	}
	if !strings.Contains(out, "usage: generate-config <output-dir>") {
		t.Fatalf("usage error must print usage, got %q", out)
	}
}

// TestMainReportsGenerationFailure runs main() in a child process with an
// output path beneath a regular file, asserting the generation failure exits
// 1 with the generate-config error prefix.
func TestMainReportsGenerationFailure(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(filepath.Clean(blocker), []byte("x"), 0644); err != nil { //nolint:gosec // test fixture under t.TempDir
		t.Fatalf("write blocker: %v", err)
	}
	code, out := runMainChild(t, "generate", filepath.Join(blocker, "out"))
	if code != 1 {
		t.Fatalf("generation failure must exit 1, got %d (output: %s)", code, out)
	}
	if !strings.Contains(out, "generate-config: ") {
		t.Fatalf("failure must report the generation error, got %q", out)
	}
}

// TestMainChildProcess is the child entry point for the re-exec tests above.
// When coverage is enabled, the parent passes the same -test.gocoverdir so
// the child's counters merge into the parent's -coverprofile.
func TestMainChildProcess(t *testing.T) {
	switch os.Getenv("GENERATE_CONFIG_CHILD_MODE") {
	case "usage":
		os.Args = []string{"generate-config"}
		main()
	case "generate":
		os.Args = []string{"generate-config", os.Getenv("GENERATE_CONFIG_CHILD_OUT")}
		main()
	default:
		t.Skip("child-only test; run via runMainChild")
	}
}

func runMainChild(t *testing.T, mode, outDir string) (int, string) {
	t.Helper()
	args := []string{"-test.run=TestMainChildProcess$"}
	if d := coverDir(); d != "" {
		args = append(args, "-test.gocoverdir="+d)
	}
	cmd := exec.Command(os.Args[0], args...) //nolint:gosec // re-executes the test binary itself
	cmd.Env = append(os.Environ(),
		"GENERATE_CONFIG_CHILD_MODE="+mode,
		"GENERATE_CONFIG_CHILD_OUT="+outDir,
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

func setArgs(t *testing.T, args []string) {
	t.Helper()
	old := os.Args
	os.Args = args
	t.Cleanup(func() { os.Args = old })
}
