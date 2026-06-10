package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/userconfig/config/security"
)

// PlatformConfig carries the loaded platform-tool manifest set for a workspace.
// It is intentionally narrow: platform-specific runtime wiring should be built
// from the loaded manifest objects rather than from ambient local state.
type PlatformConfig struct {
	Workspace     string
	ToolManifests []*ports.ToolManifest
	ToolRegistry  ports.ToolRegistry
}

// LoadPlatformConfig loads the platform tool registry for a workspace.
func LoadPlatformConfig(workspace string) (*PlatformConfig, error) {
	if strings.TrimSpace(workspace) == "" {
		return nil, fmt.Errorf("workspace path required")
	}
	manifests, err := LoadToolManifests(DefaultToolManifestDir(workspace))
	if err != nil {
		return nil, err
	}
	policy, err := security.LoadLocalToolPolicy("", workspace, StrictDecode)
	if err != nil {
		return nil, err
	}
	registry, err := BuildRegistry(manifests, policy, nil, nil)
	if err != nil {
		return nil, err
	}
	return &PlatformConfig{
		Workspace:     filepath.Clean(workspace),
		ToolManifests: manifests,
		ToolRegistry:  registry,
	}, nil
}
