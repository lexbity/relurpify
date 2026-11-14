//go:build linux || darwin

package securefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReadRegularWithinConfinesAndBoundsSource(t *testing.T) {
	workspace := t.TempDir()
	source := []byte("policy bytes\n")
	if err := os.WriteFile(filepath.Join(workspace, "policy"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})
	data, revision, err := ReadRegularWithin(root, "policy", int64(len(source)))
	if err != nil || string(data) != string(source) || revision != ComputeDigest(source) {
		t.Fatalf("read=%q revision=%q err=%v", data, revision, err)
	}
	data, revision, err = ReadRegularWithin(root, "absent", 1024)
	if err != nil || data != nil || revision != AbsentRevision {
		t.Fatal("missing source was not explicit")
	}
	for _, name := range []string{"../policy", "/etc/passwd", "nested/../policy", "."} {
		if _, _, err := ReadRegularWithin(root, name, 1024); err == nil {
			t.Fatalf("accepted unsafe path %q", name)
		}
	}
	if data, revision, err := ReadRegularWithin(root, "policy", int64(len(source)-1)); err == nil || data != nil || revision != "" {
		t.Fatal("oversized source was returned")
	}
	if _, _, err := ReadRegularWithin(root, "policy", 0); err == nil {
		t.Fatal("accepted nonpositive bound")
	}
	if err := os.Symlink("policy", filepath.Join(workspace, "link")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadRegularWithin(root, "link", 1024); err == nil {
		t.Fatal("accepted leaf symlink")
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "source"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadRegularWithin(root, "escape/source", 1024); err == nil {
		t.Fatal("followed parent symlink outside root")
	}
	if err := os.Link(filepath.Join(workspace, "policy"), filepath.Join(workspace, "hard")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"policy", "hard"} {
		if _, _, err := ReadRegularWithin(root, name, 1024); err == nil {
			t.Fatalf("accepted multiply linked source %s", name)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "absent")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read materialized a missing file")
	}
}

// A FIFO must be rejected without opening it in blocking mode.
func TestReadRegularWithinRejectsSpecialFiles(t *testing.T) {
	workspace := t.TempDir()
	if err := unix.Mkfifo(filepath.Join(workspace, "pipe"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, _, err := ReadRegularWithin(root, "pipe", 1024); err == nil {
		t.Fatal("accepted FIFO as policy source")
	}
	if _, _, err := ReadRegularWithin(root, "pipe", 1<<63-1); err == nil {
		t.Fatal("accepted overflow-prone read bound")
	}
}
