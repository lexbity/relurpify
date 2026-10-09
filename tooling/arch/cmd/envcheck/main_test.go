package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunPassesOnCleanTree(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "pkg/a.go", "package pkg\n")

	if code := run(dir); code != 0 {
		t.Fatalf("want zero exit on a clean tree, got %d", code)
	}
}

func TestRunFailsOnEnvAccess(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "pkg/a.go", "package pkg\n\nimport \"os\"\n\nvar _ = os.Getenv(\"X\")\n")

	if code := run(dir); code == 0 {
		t.Fatal("want non-zero exit for env access outside userconfig")
	}
}

func TestRunAllowsEnvAccessInUserconfig(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "userconfig/config/a.go", "package config\n\nimport \"os\"\n\nvar _ = os.Environ\n")

	if code := run(dir); code != 0 {
		t.Fatalf("want zero exit for env access inside userconfig, got %d", code)
	}
}

func TestRunReportsWalkErrors(t *testing.T) {
	if code := run(filepath.Join(t.TempDir(), "missing")); code == 0 {
		t.Fatal("want non-zero exit when the root cannot be walked")
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
