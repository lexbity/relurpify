package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var relurplintBin string

func TestMain(m *testing.M) {
	bin := filepath.Join(os.TempDir(), "relurplint-test-"+strings.ReplaceAll(filepath.Base(os.Args[0]), ".", "_"))
	cmd := exec.Command("go", "build", "-o", bin, "./app/relurplint")
	cmd.Dir = repoRoot()
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Stderr.WriteString("build failed: " + err.Error() + "\n" + string(out))
		os.Exit(1)
	}
	relurplintBin = bin
	code := m.Run()
	os.Remove(bin)
	os.Exit(code)
}

func TestCollectDiagnosticsAllEmpty(t *testing.T) {
	diags, err := collectDiagnostics("all", repoRoot())
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
	cmd.Dir = repoRoot()
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
	cmd.Dir = repoRoot()
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
	cmd.Dir = repoRoot()
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
	cmd.Dir = repoRoot()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v\noutput: %s", err, string(out))
	}
	if len(out) != 0 {
		t.Fatalf("expected empty output (no diagnostics), got: %s", string(out))
	}
}

func repoRoot() string {
	return filepath.Clean(filepath.Join("..", ".."))
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}
