package authorization

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/platform/fs"
)

func TestNormalizePathSimpleFile(t *testing.T) {
	ws := t.TempDir()
	m := testPermManager(ws)

	result, err := m.normalizePath("foo.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.ToSlash(filepath.Join(ws, "foo.txt"))
	if result != want {
		t.Fatalf("expected %q, got %q", want, result)
	}
}

func TestNormalizePathSubDirectory(t *testing.T) {
	ws := t.TempDir()
	m := testPermManager(ws)

	result, err := m.normalizePath("a/b/c.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.ToSlash(filepath.Join(ws, "a/b/c.txt"))
	if result != want {
		t.Fatalf("expected %q, got %q", want, result)
	}
}

func TestNormalizePathTraversalDotDot(t *testing.T) {
	ws := t.TempDir()
	m := testPermManager(ws)

	_, err := m.normalizePath("../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestNormalizePathAbsoluteEscapeBlocked(t *testing.T) {
	ws := t.TempDir()
	m := testPermManager(ws)

	_, err := m.normalizePath("/etc/passwd")
	if err == nil {
		t.Fatal("expected error for absolute path escape")
	}
}

func TestNormalizePathSymlinkInsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	realDir := filepath.Join(ws, "realdir")
	if err := fs.MkdirAllSecure(realDir); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(ws, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}

	m := testPermManager(ws)
	result, err := m.normalizePath("link/file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.ToSlash(filepath.Join(realDir, "file.txt"))
	if result != want {
		t.Fatalf("expected %q, got %q", want, result)
	}
}

func TestNormalizePathSymlinkEscapesWorkspace(t *testing.T) {
	ws := t.TempDir()
	outsideDir := t.TempDir()

	link := filepath.Join(ws, "evil")
	if err := os.Symlink(outsideDir, link); err != nil {
		t.Fatal(err)
	}

	m := testPermManager(ws)
	_, err := m.normalizePath("evil/passwd")
	if err == nil {
		t.Fatal("expected error for symlink escaping workspace")
	}
}

func TestNormalizePathNewFileNoSymlinks(t *testing.T) {
	ws := t.TempDir()
	m := testPermManager(ws)

	// A non-existent path under a non-existent directory — no symlinks
	// to resolve, but the path must still be within the workspace.
	result, err := m.normalizePath("newdir/newfile.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.ToSlash(filepath.Join(ws, "newdir/newfile.txt"))
	if result != want {
		t.Fatalf("expected %q, got %q", want, result)
	}
}

func TestNormalizePathWorkspaceBoundaryExact(t *testing.T) {
	ws := t.TempDir()
	m := testPermManager(ws)

	result, err := m.normalizePath(ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.ToSlash(result) != filepath.ToSlash(ws) {
		t.Fatalf("expected %q, got %q", ws, result)
	}
}

func TestNormalizePathEmptyString(t *testing.T) {
	ws := t.TempDir()
	m := testPermManager(ws)

	_, err := m.normalizePath("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestNormalizePathSymlinkChainsResolved(t *testing.T) {
	ws := t.TempDir()
	targetDir := filepath.Join(ws, "target")
	if err := fs.MkdirAllSecure(targetDir); err != nil {
		t.Fatal(err)
	}
	link1 := filepath.Join(ws, "link1")
	link2 := filepath.Join(ws, "link2")
	if err := os.Symlink(targetDir, link1); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(link1, link2); err != nil {
		t.Fatal(err)
	}

	m := testPermManager(ws)
	result, err := m.normalizePath("link2/data.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.ToSlash(filepath.Join(targetDir, "data.txt"))
	if result != want {
		t.Fatalf("expected %q, got %q", want, result)
	}
}

func TestNormalizePathDotInMiddle(t *testing.T) {
	ws := t.TempDir()
	m := testPermManager(ws)

	result, err := m.normalizePath("./foo/./bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.ToSlash(filepath.Join(ws, "foo/bar"))
	if result != want {
		t.Fatalf("expected %q, got %q", want, result)
	}
}

// testPermManager creates a PermissionManager with basePath set to the given
// workspace directory, with minimal permissions for initialization.
func testPermManager(ws string) *PermissionManager {
	audit := policy.NewInMemoryAuditLogger(100)
	declared := &permissions.PermissionSet{
		Executables: []permissions.ExecutablePermission{
			{Binary: "echo"},
		},
	}
	m, err := NewPermissionManager(ws, declared, audit, nil)
	if err != nil {
		panic(err)
	}
	return m
}
