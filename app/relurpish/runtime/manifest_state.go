package runtime

import (
	"fmt"
	"os"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// SaveManifestSpecWithBackup writes the manifest spec to path after snapshotting
// the previous file into relurpify_cfg/backups.
func SaveManifestSpecWithBackup(path string, spec *config.ManifestSpec) (string, error) {
	if path == "" {
		return "", fmt.Errorf("manifest path required")
	}
	if spec == nil {
		return "", fmt.Errorf("manifest required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	backup, err := config.CreateTimestampedBackup(path)
	if err != nil {
		return "", err
	}
	out := struct {
		APIVersion string                  `yaml:"apiVersion"`
		Kind       string                  `yaml:"kind"`
		Metadata   config.DocumentMetadata `yaml:"metadata"`
		Spec       *config.ManifestSpec    `yaml:"spec"`
	}{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentManifest",
		Metadata:   config.DocumentMetadata{Name: "agent"},
		Spec:       spec,
	}
	if err := config.SaveYAML(path, out); err != nil {
		return "", fmt.Errorf("write manifest: %w", err)
	}
	return backup, nil
}
