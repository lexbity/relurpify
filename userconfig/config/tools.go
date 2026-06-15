package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	configmanifest "codeburg.org/lexbit/relurpify/capability/ports"
)

// DefaultToolManifestDir returns the canonical tool manifest directory.
func DefaultToolManifestDir(workspace string) string {
	return filepath.Join(workspace, "relurpify_cfg", "tools")
}

// LoadToolManifests loads every tool definition beneath the provided directory
// in deterministic order.
func LoadToolManifests(dir string) ([]*configmanifest.ToolManifest, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("tool manifest directory required")
	}
	paths, err := collectToolManifestPaths(dir)
	if err != nil {
		return nil, err
	}
	out := make([]*configmanifest.ToolManifest, 0, len(paths))
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
func LoadToolManifest(path string) (*configmanifest.ToolManifest, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var manifest configmanifest.ToolManifest
	decl, err := DecodeWithSchema(path, data, NewSchemaRegistry(), &manifest)
	if err != nil {
		return nil, err
	}
	if decl.Kind != schemaKindTool {
		return nil, &SchemaError{
			Path:   path,
			Line:   decl.Line,
			Schema: decl.String(),
			Err:    fmt.Errorf("tool manifest must use relurpify/%s/v1 or relurpify/%s/v2, got %s", schemaKindTool, schemaKindTool, decl.String()),
		}
	}
	if decl.Version != 1 && decl.Version != 2 {
		return nil, &SchemaError{
			Path:   path,
			Line:   decl.Line,
			Schema: decl.String(),
			Err:    fmt.Errorf("tool manifest version must be 1 or 2, got %d", decl.Version),
		}
	}
	manifest.SourcePath = path
	manifest.CanonicalName = configmanifest.NormalizeToolName(manifest.Name)
	manifest.Name = manifest.CanonicalName
	manifest.Family = configmanifest.NormalizeToolName(manifest.Family)
	for i, intent := range manifest.Intent {
		manifest.Intent[i] = configmanifest.NormalizeToolName(intent)
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
