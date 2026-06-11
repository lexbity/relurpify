package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

var relurplintBin string
var testRepoRoot string

func TestMain(m *testing.M) {
	cwd, err := os.Getwd()
	if err != nil {
		_, _ = os.Stderr.WriteString("getwd: " + err.Error() + "\n")
		os.Exit(1)
	}
	testRepoRoot = filepath.Clean(filepath.Join(cwd, "..", ".."))

	bin := filepath.Join(os.TempDir(), "relurplint-test-"+strings.ReplaceAll(filepath.Base(os.Args[0]), ".", "_"))
	goPath, err := exec.LookPath("go")
	if err != nil {
		_, _ = os.Stderr.WriteString("go not found in PATH")
		os.Exit(1)
	}
	cmd := &exec.Cmd{
		Path: goPath,
		Args: []string{goPath, "build", "-o", filepath.Clean(bin), "./app/relurplint"},
	}
	cmd.Dir = testRepoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		_, _ = os.Stderr.WriteString("build failed: " + err.Error() + "\n" + string(out))
		os.Exit(1)
	}
	relurplintBin = bin
	code := m.Run()
	_ = os.Remove(bin)
	os.Exit(code)
}

func TestCollectDiagnosticsAllEmpty(t *testing.T) {
	diags, err := collectDiagnostics("all", testhelper.RepoRoot(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics (no checks registered), got %d", len(diags))
	}
}

func TestSelectedAll(t *testing.T) {
	checks, err := selected("all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(checks) != 4 {
		t.Fatalf("expected 4 checks (config, tools, recipes, prompts), got %d: %v", len(checks), checks)
	}
}

func TestSelectedUnknown(t *testing.T) {
	_, err := selected("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown check")
	}
	if !strings.Contains(err.Error(), "unknown check") {
		t.Fatalf("expected 'unknown check' error, got: %v", err)
	}
}

func TestSelectedCSVWithUnknown(t *testing.T) {
	_, err := selected("config,bogus")
	if err == nil {
		t.Fatal("expected error for unknown check in CSV")
	}
}

func TestCLIUnknownCheck(t *testing.T) {
	cmd := exec.Command(relurplintBin, "--check", "nonexistent")
	cmd.Dir = testhelper.RepoRoot(t)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for unknown check")
	}
	if exitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %d", exitCode(err))
	}
	if strings.Contains(string(out), "diagnostics") {
		t.Fatalf("unexpected JSON output on error: %s", string(out))
	}
}

func TestCLIAllJSON(t *testing.T) {
	cmd := exec.Command(relurplintBin, "--check", "all", "--format", "json")
	cmd.Dir = testhelper.RepoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, string(out))
	}

	var parsed struct {
		Diagnostics []any `json:"diagnostics"`
		Summary     struct {
			Total    int `json:"total"`
			Errors   int `json:"errors"`
			Warnings int `json:"warnings"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, string(out))
	}
	if parsed.Summary.Total != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", parsed.Summary.Total)
	}
}

func TestCLIHelp(t *testing.T) {
	cmd := exec.Command(relurplintBin, "--help")
	cmd.Dir = testhelper.RepoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "relurplint") {
		t.Fatalf("expected help text, got: %s", string(out))
	}
}

func TestCLIAllText(t *testing.T) {
	cmd := exec.Command(relurplintBin, "--check", "all", "--format", "text")
	cmd.Dir = testhelper.RepoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, string(out))
	}
	if len(out) != 0 {
		t.Fatalf("expected empty output (no diagnostics), got: %s", string(out))
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	exitErr := &exec.ExitError{}
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
