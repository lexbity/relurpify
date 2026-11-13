// Package securefile provides the secure file primitives the config domain
// needs to persist configuration and runtime state.
package securefile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// FileTransaction represents an atomic file write transaction with
// revision checking and safe rollback (P04/D-17, P05).
type FileTransaction struct {
	// Destination is the target file path.
	Destination string
	// Source is the current content to replace.
	Source []byte
	// ExpectedDigest is the SHA-256 hash of the current file content.
	// If non-empty, the transaction will fail if the file doesn't match.
	ExpectedDigest string
	// TempPath is the temporary file path for atomic write.
	TempPath string
	// BackupPath is where the original file is backed up (optional).
	BackupPath string
}

// NewFileTransaction creates a new file transaction.
func NewFileTransaction(dest string) *FileTransaction {
	return &FileTransaction{
		Destination: dest,
	}
}

// WithExpectedDigest sets the expected current digest for CAS check.
func (t *FileTransaction) WithExpectedDigest(digest string) *FileTransaction {
	t.ExpectedDigest = digest
	return t
}

// WithBackup enables backup of the original file.
func (t *FileTransaction) WithBackup(backupPath string) *FileTransaction {
	t.BackupPath = backupPath
	return t
}

// ComputeDigest computes the SHA-256 digest of content.
func ComputeDigest(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

// CurrentDigest returns the current digest of the destination file.
func (t *FileTransaction) CurrentDigest() (string, error) {
	content, err := os.ReadFile(t.Destination)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // File doesn't exist, no digest
		}
		return "", fmt.Errorf("read current file: %w", err)
	}
	return ComputeDigest(content), nil
}

// Validate checks if the transaction can proceed (file matches expected digest).
func (t *FileTransaction) Validate() error {
	if t.ExpectedDigest == "" {
		return nil // No check required
	}
	
	current, err := t.CurrentDigest()
	if err != nil {
		return err
	}
	
	if t.ExpectedDigest != current {
		return fmt.Errorf("revision conflict: expected %s, got %s",
			t.ExpectedDigest, current)
	}
	
	return nil
}

// Execute performs the atomic write transaction.
// Returns the new digest or an error.
func (t *FileTransaction) Execute() (string, error) {
	// Validate first
	if err := t.Validate(); err != nil {
		return "", err
	}

	// Create temp file in same directory
	dir := filepath.Dir(t.Destination)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, filepath.Base(t.Destination)+".tmp")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	t.TempPath = tmpFile.Name()
	defer func() {
		if tmpFile != nil {
			_ = tmpFile.Close()
		}
		_ = os.Remove(t.TempPath)
	}()

	// Backup existing file if requested
	if t.BackupPath != "" && t.ExpectedDigest != "" {
		current, _ := t.CurrentDigest()
		if current != "" {
			// Copy current to backup
			if err := copyFile(t.Destination, t.BackupPath); err != nil {
				return "", fmt.Errorf("create backup: %w", err)
			}
		}
	}

	// Write new content
	if _, err := tmpFile.Write(t.Source); err != nil {
		return "", fmt.Errorf("write temp file: %w", err)
	}

	// Ensure file is on disk
	if err := tmpFile.Sync(); err != nil {
		return "", fmt.Errorf("sync temp file: %w", err)
	}

	// Close before rename
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}
	tmpFile = nil

	// Check for symlink attacks
	if err := validateNoSymlinkAttack(t.TempPath, t.Destination); err != nil {
		return "", err
	}

	// Atomic rename
	if err := os.Rename(t.TempPath, t.Destination); err != nil {
		return "", fmt.Errorf("rename: %w", err)
	}

	// Set permissions
	if err := os.Chmod(t.Destination, 0600); err != nil {
		// Try to rollback on permission error
		_ = os.Rename(t.Destination, t.TempPath)
		return "", fmt.Errorf("set permissions: %w", err)
	}

	// Compute new digest
	newDigest := ComputeDigest(t.Source)
	return newDigest, nil
}

// copyFile copies src to dst, creating directories as needed.
func copyFile(src, dst string) error {
	content, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Use secure write
	return WriteFileSecure(dst, content)
}

// validateNoSymlinkAttack checks that neither path is a symlink and
// that the rename target is not a symlink.
func validateNoSymlinkAttack(tempPath, dest string) error {
	// Check temp file
	info, err := os.Lstat(tempPath)
	if err != nil {
		return fmt.Errorf("lstat temp: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("temp file is a symlink")
	}

	// Check destination
	info, err = os.Lstat(dest)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("lstat dest: %w", err)
		}
		// Destination doesn't exist, that's fine
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("destination is a symlink")
	}

	// On Unix, also check if they're on the same filesystem
	var statTemp, statDest syscall.Stat_t
	if err := syscall.Stat(tempPath, &statTemp); err != nil {
		return fmt.Errorf("stat temp: %w", err)
	}
	if err := syscall.Stat(dest, &statDest); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat dest: %w", err)
		}
		// Destination doesn't exist, can't check dev
		return nil
	}
	if statTemp.Dev != statDest.Dev {
		return fmt.Errorf("files are on different devices, atomic rename not guaranteed")
	}

	return nil
}

// ConfinedWrite performs a confined, revision-checked atomic write.
// The path must be a regular file within its parent directory.
func ConfinedWrite(dest string, content []byte, expectedDigest string) (string, error) {
	txn := NewFileTransaction(dest).
		WithExpectedDigest(expectedDigest)
	if err := txn.Validate(); err != nil {
		return "", err
	}
	return txn.Execute()
}

// MultiFileTransaction handles atomic writes of multiple files.
// This is used for configuration transactions where multiple files
// need to be updated atomically (P05).
type MultiFileTransaction struct {
	// Files is the set of files to write.
	Files map[string][]byte
	// ExpectedDigests maps file paths to their expected current digests.
	ExpectedDigests map[string]string
	// Backups is where to back up original files.
	Backups map[string]string
	// Writes that have been performed (for rollback).
	performed []string
	// Temp files created.
	tempFiles []string
}

// NewMultiFileTransaction creates a new multi-file transaction.
func NewMultiFileTransaction() *MultiFileTransaction {
	return &MultiFileTransaction{
		Files:          make(map[string][]byte),
		ExpectedDigests: make(map[string]string),
		Backups:        make(map[string]string),
		performed:      make([]string, 0),
		tempFiles:     make([]string, 0),
	}
}

// AddFile adds a file to the transaction.
func (t *MultiFileTransaction) AddFile(path string, content []byte) *MultiFileTransaction {
	t.Files[path] = content
	return t
}

// WithExpectedDigest sets the expected digest for a file.
func (t *MultiFileTransaction) WithExpectedDigest(path, digest string) *MultiFileTransaction {
	t.ExpectedDigests[path] = digest
	return t
}

// WithBackup sets the backup path for a file.
func (t *MultiFileTransaction) WithBackup(path, backupPath string) *MultiFileTransaction {
	t.Backups[path] = backupPath
	return t
}

// Execute performs the transaction.
// If any write fails, it attempts to rollback all performed writes.
func (t *MultiFileTransaction) Execute() error {
	for path, content := range t.Files {
		// Validate
		expectedDigest := t.ExpectedDigests[path]
		currentDigest, err := computeCurrentDigest(path)
		if err != nil {
			return fmt.Errorf("get current digest for %s: %w", path, err)
		}
		if expectedDigest != "" && expectedDigest != currentDigest {
			return fmt.Errorf("revision conflict for %s: expected %s, got %s",
				path, expectedDigest, currentDigest)
		}

		// Backup if requested
		if backupPath, ok := t.Backups[path]; ok {
			if err := backupFile(path, backupPath); err != nil {
				return fmt.Errorf("backup %s: %w", path, err)
			}
		}

		// Perform atomic write
		txn := NewFileTransaction(path).WithExpectedDigest(expectedDigest)
		txn.Source = content
		if _, err := txn.Execute(); err != nil {
			// Rollback performed writes
			_ = t.Rollback()
			return fmt.Errorf("write %s: %w", path, err)
		}

		t.performed = append(t.performed, path)
	}

	return nil
}

// Rollback attempts to rollback all performed writes.
func (t *MultiFileTransaction) Rollback() error {
	// Reverse order for rollback
	var errors []error
	for i := len(t.performed) - 1; i >= 0; i-- {
		path := t.performed[i]
		if backupPath, ok := t.Backups[path]; ok {
			if err := os.Rename(backupPath, path); err != nil {
				errors = append(errors, fmt.Errorf("rollback %s: %w", path, err))
			}
		} else {
			// No backup, try to delete
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				errors = append(errors, fmt.Errorf("rollback remove %s: %w", path, err))
			}
		}
	}

	if len(errors) > 0 {
		// Return a combined error
		var msg strings.Builder
		msg.WriteString("rollback failed:")
		for _, err := range errors {
			msg.WriteString(fmt.Sprintf("\n  - %v", err))
		}
		return fmt.Errorf(msg.String())
	}

	return nil
}

// computeCurrentDigest computes the digest of a file or returns empty if it doesn't exist.
func computeCurrentDigest(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return ComputeDigest(content), nil
}

// backupFile creates a backup of a file.
func backupFile(src, dst string) error {
	// Ensure backup directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}
	return copyFile(src, dst)
}
