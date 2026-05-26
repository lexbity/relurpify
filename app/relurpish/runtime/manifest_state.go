package runtime

import (
	"fmt"
	"os"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
)

// SaveAgentManifestWithBackup writes the manifest to path after snapshotting
// the previous file into relurpify_cfg/backups.
func SaveAgentManifestWithBackup(path string, m *cfgload.AgentManifest) (string, error) {
	if path == "" {
		return "", fmt.Errorf("manifest path required")
	}
	if m == nil {
		return "", fmt.Errorf("manifest required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	backup, err := cfgload.CreateTimestampedBackup(path)
	if err != nil {
		return "", err
	}
	if err := cfgload.SaveAgentManifest(path, m); err != nil {
		return "", err
	}
	return backup, nil
}
