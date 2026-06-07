package runtime

import (
	"fmt"
	"os"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// SaveAgentManifestWithBackup writes the manifest to path after snapshotting
// the previous file into relurpify_cfg/backups.
func SaveAgentManifestWithBackup(path string, m *config.AgentManifest) (string, error) {
	if path == "" {
		return "", fmt.Errorf("manifest path required")
	}
	if m == nil {
		return "", fmt.Errorf("manifest required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	backup, err := config.CreateTimestampedBackup(path)
	if err != nil {
		return "", err
	}
	if err := config.SaveAgentManifest(path, m); err != nil {
		return "", err
	}
	return backup, nil
}
