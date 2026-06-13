package config

import (
	"fmt"
	"os"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

// SaveDocumentWithBackup writes the document envelope to path after snapshotting
// the previous file into relurpify_cfg/backups.
func SaveDocumentWithBackup(path string, doc *Document) (string, error) {
	if path == "" {
		return "", fmt.Errorf("document path required")
	}
	if doc == nil {
		return "", fmt.Errorf("document required")
	}
	if err := os.MkdirAll(filepath.Dir(path), fs.PublicDirMode); err != nil { // public: document parent dir
		return "", err
	}
	backup, err := CreateTimestampedBackup(path)
	if err != nil {
		return "", err
	}
	if err := SaveYAML(path, doc); err != nil {
		return "", fmt.Errorf("write document: %w", err)
	}
	return backup, nil
}
