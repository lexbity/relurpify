package templates

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	relurpifyfs "codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/userconfig/templates/embedfs"
)

// GenerateConfig writes the embedded workspace template tree to the given
// output directory, mirroring its layout 1:1. It is the single producer of
// the checked-in relurpify_cfg/ tree.
// Source: embedfs.DefaultFS() subtree "workspace".
func GenerateConfig(output string) error {
	output = filepath.Clean(output)
	if output == "." || output == string(filepath.Separator) {
		return fmt.Errorf("refusing to generate config into %q", output)
	}
	efs := embedfs.DefaultFS()
	if err := os.RemoveAll(output); err != nil {
		return fmt.Errorf("clean output dir %q: %w", output, err)
	}
	if err := relurpifyfs.MkdirAllSecure(output); err != nil {
		return fmt.Errorf("create output dir %q: %w", output, err)
	}
	return fs.WalkDir(efs, "workspace", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "workspace" {
			return nil
		}
		rel := strings.TrimPrefix(path, "workspace/")
		target := filepath.Join(output, rel)
		if d.IsDir() {
			return relurpifyfs.MkdirAllSecure(target)
		}
		data, err := fs.ReadFile(efs, path)
		if err != nil {
			return err
		}
		if err := relurpifyfs.MkdirAllSecure(filepath.Dir(target)); err != nil {
			return err
		}
		return relurpifyfs.WriteFileSecure(target, data)
	})
}
