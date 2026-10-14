package main

import (
	"bytes"
	"flag"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/userconfig/templates/embedfs"
)

// TestMainMaterialisesEmbeddedTemplates runs main() in-process on the success
// path (which returns without exiting) and asserts the embedded workspace
// tree is mirrored 1:1 under <out>/workspace.
func TestMainMaterialisesEmbeddedTemplates(t *testing.T) {
	outDir := t.TempDir()
	setArgs(t, []string{"driftcheck", outDir})
	stdout := captureStdout(t)

	main()

	if got := stdout(); !strings.Contains(got, "materialised embedded templates to") {
		t.Fatalf("main must report the materialisation target, got stdout %q", got)
	}

	src := embedfs.DefaultFS()
	err := fs.WalkDir(src, "workspace", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "workspace" {
			return nil
		}
		dst := filepath.Join(outDir, path)
		info, statErr := os.Stat(filepath.Clean(dst))
		if statErr != nil {
			t.Fatalf("materialised tree is missing %s: %v", path, statErr)
		}
		if d.IsDir() != info.IsDir() {
			t.Fatalf("%s: directory-ness mismatch: embedded isDir=%v materialised isDir=%v", path, d.IsDir(), info.IsDir())
		}
		if d.IsDir() {
			return nil
		}
		want, readErr := fs.ReadFile(src, path)
		if readErr != nil {
			t.Fatalf("read embedded %s: %v", path, readErr)
		}
		gotData, readErr := os.ReadFile(filepath.Clean(dst))
		if readErr != nil {
			t.Fatalf("read materialised %s: %v", path, readErr)
		}
		if string(want) != string(gotData) {
			t.Fatalf("%s: materialised content differs from embedded template", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded templates: %v", err)
	}
}

// TestMainUsageExitsNonZero runs main() in a child process sharing the
// coverage directory, asserting the missing-argument usage error exits 1.
func TestMainUsageExitsNonZero(t *testing.T) {
	code, out := runMainChild(t, "usage", "")
	if code != 1 {
		t.Fatalf("usage error must exit 1, got %d (output: %s)", code, out)
	}
	if !strings.Contains(out, "usage: driftcheck <output-dir>") {
		t.Fatalf("usage error must print usage, got %q", out)
	}
}

// TestMainReportsMaterialisationFailure runs main() in a child process with
// an output path beneath a regular file, asserting the walk/write failure
// exits 1 with the materialisation error.
func TestMainReportsMaterialisationFailure(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(filepath.Clean(blocker), []byte("x"), 0644); err != nil { //nolint:gosec // test fixture under t.TempDir
		t.Fatalf("write blocker: %v", err)
	}
	code, out := runMainChild(t, "materialise", filepath.Join(blocker, "out"))
	if code != 1 {
		t.Fatalf("materialisation failure must exit 1, got %d (output: %s)", code, out)
	}
	if !strings.Contains(out, "materialize workspace templates") {
		t.Fatalf("failure must report the materialisation error, got %q", out)
	}
}

// TestMainChildProcess is the child entry point for the re-exec tests above.
// It runs main() against env-selected arguments; main's os.Exit then becomes
// the child process exit code. When coverage is enabled, the parent passes
// the same -test.gocoverdir so the child's counters merge into the parent's
// -coverprofile.
func TestMainChildProcess(t *testing.T) {
	switch os.Getenv("DRIFTCHECK_CHILD_MODE") {
	case "usage":
		os.Args = []string{"driftcheck"}
		main()
	case "materialise":
		os.Args = []string{"driftcheck", os.Getenv("DRIFTCHECK_CHILD_OUT")}
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
		"DRIFTCHECK_CHILD_MODE="+mode,
		"DRIFTCHECK_CHILD_OUT="+outDir,
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

// captureStdout redirects os.Stdout and returns a single-shot reader for
// everything written so far. It must be called exactly once per test.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = buf.ReadFrom(r)
		close(done)
	}()
	old := os.Stdout
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = old
		_ = w.Close()
		<-done
		_ = r.Close()
	})
	return func() string {
		_ = w.Close()
		<-done
		return buf.String()
	}
}
