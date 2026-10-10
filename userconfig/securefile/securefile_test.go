package securefile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileSecureModes(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "nested", "config.yaml")

	if err := MkdirAllSecure(filepath.Dir(target)); err != nil {
		t.Fatalf("MkdirAllSecure: %v", err)
	}
	info, err := os.Stat(filepath.Dir(target))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("dir mode = %o, want 700", got)
	}

	if err := WriteFileSecure(target, []byte("key: value")); err != nil {
		t.Fatalf("WriteFileSecure: %v", err)
	}
	info, err = os.Stat(target)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode = %o, want 600", got)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "key: value" {
		t.Errorf("readback = %q, err %v", data, err)
	}
}
