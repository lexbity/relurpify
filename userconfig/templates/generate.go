package templates

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	relurpifyfs "codeburg.org/lexbit/relurpify/platform/fs"
)

// GenerateWorkspaceTemplates copies the canonical workspace template tree to
// the requested output directory.
func GenerateWorkspaceTemplates(output string) error {
	output = filepath.Clean(output)
	if output == "." || output == string(filepath.Separator) {
		return fmt.Errorf("refusing to generate templates into %q", output)
	}
	source := filepath.Join(repoRoot(), "templates", "workspace")
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("locate workspace templates: %w", err)
	}
	if err := os.RemoveAll(output); err != nil {
		return err
	}
	if err := relurpifyfs.MkdirAllSecure(output); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(output, rel)
		if d.IsDir() {
			return relurpifyfs.MkdirAllSecure(target)
		}
		data, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}
		if err := relurpifyfs.MkdirAllSecure(filepath.Dir(target)); err != nil {
			return err
		}
		return relurpifyfs.WriteFileSecure(target, data)
	})
}
