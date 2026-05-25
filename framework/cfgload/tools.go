package cfgload

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// DefaultToolManifestDir returns the canonical tool manifest directory.
func DefaultToolManifestDir(workspace string) string {
	return filepath.Join(workspace, "relurpify_cfg", "tools")
}

// LoadToolManifests loads every tool definition beneath the provided directory
// in deterministic order.
func LoadToolManifests(dir string) ([]*contracts.ToolManifest, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("tool manifest directory required")
	}
	paths, err := collectToolManifestPaths(dir)
	if err != nil {
		return nil, err
	}
	out := make([]*contracts.ToolManifest, 0, len(paths))
	for _, path := range paths {
		manifest, err := LoadToolManifest(path)
		if err != nil {
			return nil, err
		}
		out = append(out, manifest)
	}
	return out, nil
}

// LoadToolManifest loads a single .tool.yaml file.
func LoadToolManifest(path string) (*contracts.ToolManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest contracts.ToolManifest
	decl, err := DecodeWithSchema(path, data, NewSchemaRegistry(), &manifest)
	if err != nil {
		return nil, err
	}
	if decl.Kind != "tool" {
		return nil, &SchemaError{
			Path:   path,
			Line:   decl.Line,
			Schema: decl.String(),
			Err:    fmt.Errorf("tool manifest must use relurpify/tool/v1"),
		}
	}
	manifest.SourcePath = path
	manifest.CanonicalName = contracts.NormalizeToolName(manifest.Name)
	manifest.Name = manifest.CanonicalName
	manifest.Family = contracts.NormalizeToolName(manifest.Family)
	for i, intent := range manifest.Intent {
		manifest.Intent[i] = contracts.NormalizeToolName(intent)
	}
	if err := validateToolManifest(path, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func collectToolManifestPaths(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(dir, name)
		if entry.IsDir() {
			subpaths, err := collectToolManifestPaths(path)
			if err != nil {
				return nil, err
			}
			paths = append(paths, subpaths...)
			continue
		}
		if strings.HasSuffix(name, ".tool.yaml") {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}
