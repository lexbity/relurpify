package langdetect

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectEmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	got := Detect(dir)

	if !got.IsEmpty() {
		t.Fatalf("expected empty detection, got %#v", got)
	}
	if len(got.Detected()) != 0 {
		t.Fatalf("expected no detected languages, got %v", got.Detected())
	}
}

func TestDetectGoMod(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n")

	got := Detect(dir)

	want := WorkspaceLanguages{Go: true}
	if got != want {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestDetectPackageJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{}\n")

	got := Detect(dir)

	want := WorkspaceLanguages{JS: true}
	if got != want {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestDetectMultipleRootIndicators(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n")
	writeFile(t, dir, "Cargo.toml", "[package]\nname = \"test\"\n")

	got := Detect(dir)

	want := WorkspaceLanguages{Go: true, Rust: true}
	if got != want {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestDetectGoFileInSubdirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "src")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	writeFile(t, subdir, "main.go", "package main\n")

	got := Detect(dir)

	want := WorkspaceLanguages{Go: true}
	if got != want {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestDetectPythonIndicators(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml", "[project]\nname = \"example\"\n")

	subdir := filepath.Join(dir, "app")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	writeFile(t, subdir, "worker.py", "print('hi')\n")

	got := Detect(dir)

	want := WorkspaceLanguages{Python: true}
	if got != want {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestDetectDepthLimit(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "one", "two", "three")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir deep tree: %v", err)
	}
	writeFile(t, deep, "main.go", "package main\n")

	got := Detect(dir)

	if !got.IsEmpty() {
		t.Fatalf("expected depth-limited scan to ignore deep file, got %#v", got)
	}
}

func TestDetectNonExistentPath(t *testing.T) {
	got := Detect(filepath.Join(t.TempDir(), "missing"))

	if !got.IsEmpty() {
		t.Fatalf("expected empty detection for missing path, got %#v", got)
	}
}

func TestDetectBlankPath(t *testing.T) {
	got := Detect("   ")

	if !got.IsEmpty() {
		t.Fatalf("expected empty detection for blank path, got %#v", got)
	}
}

func TestWorkspaceLanguagesDetectedStableOrder(t *testing.T) {
	langs := WorkspaceLanguages{JS: true, Go: true, Rust: true, Python: true}

	got := langs.Detected()
	want := []string{"go", "python", "rust", "js"}

	if len(got) != len(want) {
		t.Fatalf("expected %d languages, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected stable order %v, got %v", want, got)
		}
	}
}

func TestWorkspaceLanguagesIsEmpty(t *testing.T) {
	if !((WorkspaceLanguages{}).IsEmpty()) {
		t.Fatal("expected zero value to be empty")
	}
	if (WorkspaceLanguages{Go: true}).IsEmpty() {
		t.Fatal("expected non-zero value to be non-empty")
	}
}

func TestDetectEntryCap(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < defaultMaxEntries+1; i++ {
		writeFile(t, dir, fmt.Sprintf("noise_%03d.txt", i), "x")
	}

	got := Detect(dir)

	if !got.IsEmpty() {
		t.Fatalf("expected cap-limited scan to stay empty, got %#v", got)
	}
}

func BenchmarkDetect(b *testing.B) {
	dir := b.TempDir()
	mustWrite := func(dir, name, content string) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			b.Fatalf("write file %s/%s: %v", dir, name, err)
		}
	}
	mustWrite(dir, "go.mod", "module example.com/test\n")
	mustWrite(filepath.Join(dir, "src"), "main.go", "package main\n")
	mustWrite(filepath.Join(dir, "pkg"), "util.py", "print('hi')\n")
	mustWrite(filepath.Join(dir, "web"), "package.json", "{}\n")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Detect(dir)
	}
}

func writeFile(t testing.TB, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s/%s: %v", dir, name, err)
	}
}
