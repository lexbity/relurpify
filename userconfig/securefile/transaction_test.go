package securefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileTransactionExecuteNewFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "nested", "config.yaml")
	txn := NewFileTransaction(dest)
	txn.Source = []byte("key: value\n")

	digest, err := txn.Execute()
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if want := ComputeDigest(txn.Source); digest != want {
		t.Fatalf("digest = %s, want %s", digest, want)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("dest mode = %o, want 600", got)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != string(txn.Source) {
		t.Errorf("readback = %q, err %v", data, err)
	}
}

func TestFileTransactionExecuteWithExpectedDigest(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(dest, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldDigest := ComputeDigest([]byte("old"))

	txn := NewFileTransaction(dest).WithExpectedDigest(oldDigest)
	txn.Source = []byte("new")
	if _, err := txn.Execute(); err != nil {
		t.Fatalf("Execute with matching digest: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "new" {
		t.Errorf("readback = %q, err %v", data, err)
	}
}

func TestFileTransactionExecuteRejectsRevisionConflict(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(dest, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	txn := NewFileTransaction(dest).WithExpectedDigest(ComputeDigest([]byte("other")))
	txn.Source = []byte("new")
	if _, err := txn.Execute(); err == nil || !strings.Contains(err.Error(), "revision conflict") {
		t.Fatalf("Execute = %v, want revision conflict", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "old" {
		t.Errorf("dest mutated on conflict: %q, err %v", data, err)
	}
}

func TestFileTransactionValidate(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(dest, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	txn := NewFileTransaction(dest)
	if err := txn.Validate(); err != nil {
		t.Fatalf("Validate without expected digest: %v", err)
	}
	if err := txn.WithExpectedDigest(ComputeDigest([]byte("content"))).Validate(); err != nil {
		t.Fatalf("Validate with matching digest: %v", err)
	}
	if err := txn.WithExpectedDigest(ComputeDigest([]byte("other"))).Validate(); err == nil {
		t.Fatal("Validate with mismatched digest succeeded")
	}
}

func TestFileTransactionCurrentDigestMissingFile(t *testing.T) {
	txn := NewFileTransaction(filepath.Join(t.TempDir(), "absent.yaml"))
	digest, err := txn.CurrentDigest()
	if err != nil {
		t.Fatalf("CurrentDigest: %v", err)
	}
	if digest != "" {
		t.Fatalf("digest = %q, want empty", digest)
	}
}

func TestFileTransactionBackup(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "config.yaml")
	backup := filepath.Join(dir, "backup", "config.yaml")
	if err := os.WriteFile(dest, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	txn := NewFileTransaction(dest).
		WithExpectedDigest(ComputeDigest([]byte("old"))).
		WithBackup(backup)
	txn.Source = []byte("new")
	if _, err := txn.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(data) != "old" {
		t.Errorf("backup = %q, want old content", data)
	}
}

func TestFileTransactionRejectsDestSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "config.yaml")
	if err := os.Symlink(target, dest); err != nil {
		t.Fatal(err)
	}
	txn := NewFileTransaction(dest)
	txn.Source = []byte("new")
	if _, err := txn.Execute(); err == nil {
		t.Fatal("Execute through destination symlink succeeded")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "original" {
		t.Errorf("symlink target mutated: %q, err %v", data, err)
	}
}

func TestConfinedWrite(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "config.yaml")
	if _, err := ConfinedWrite(dest, []byte("v1"), ""); err != nil {
		t.Fatalf("ConfinedWrite create: %v", err)
	}
	digest, err := ConfinedWrite(dest, []byte("v2"), ComputeDigest([]byte("v1")))
	if err != nil {
		t.Fatalf("ConfinedWrite replace: %v", err)
	}
	if want := ComputeDigest([]byte("v2")); digest != want {
		t.Fatalf("digest = %s, want %s", digest, want)
	}
	if _, err := ConfinedWrite(dest, []byte("v3"), ComputeDigest([]byte("stale"))); err == nil {
		t.Fatal("ConfinedWrite with stale digest succeeded")
	}
}

func TestMultiFileTransactionExecute(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.yaml")
	b := filepath.Join(dir, "b.yaml")
	txn := NewMultiFileTransaction().
		AddFile(a, []byte("A")).
		AddFile(b, []byte("B"))
	if err := txn.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for path, want := range map[string]string{a: "A", b: "B"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Errorf("%s = %q, err %v; want %q", path, data, err, want)
		}
	}
}

func TestMultiFileTransactionRevisionConflictRollsBack(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.yaml")
	b := filepath.Join(dir, "b.yaml")

	txn := NewMultiFileTransaction().
		AddFile(a, []byte("A")).
		AddFile(b, []byte("B")).
		WithExpectedDigest(b, ComputeDigest([]byte("stale")))
	if err := txn.Execute(); err == nil {
		t.Fatal("Execute with stale digest succeeded")
	}
	// Neither file may survive: b was rejected, a was rolled back if written.
	for _, path := range []string{a, b} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s exists after rollback (err %v)", path, err)
		}
	}
}

func TestMultiFileTransactionBackupRestoredOnRollback(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.yaml")
	b := filepath.Join(dir, "b.yaml")
	backupA := filepath.Join(dir, "bk", "a.yaml")
	backupB := filepath.Join(dir, "bk", "b.yaml")
	if err := os.WriteFile(a, []byte("A-orig"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("B-orig"), 0o600); err != nil {
		t.Fatal(err)
	}

	txn := NewMultiFileTransaction().
		AddFile(a, []byte("A-new")).
		AddFile(b, []byte("B-new")).
		WithBackup(a, backupA).
		WithBackup(b, backupB).
		WithExpectedDigest(b, ComputeDigest([]byte("stale")))
	if err := txn.Execute(); err == nil {
		t.Fatal("Execute with stale digest succeeded")
	}
	// Originals must survive whether or not a was written before the conflict.
	for path, want := range map[string]string{a: "A-orig", b: "B-orig"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Errorf("%s = %q, err %v; want %q", path, data, err, want)
		}
	}
}

func TestMultiFileTransactionRollbackReportsErrors(t *testing.T) {
	txn := NewMultiFileTransaction()
	if err := txn.Rollback(); err != nil {
		t.Fatalf("Rollback on empty transaction: %v", err)
	}
}
