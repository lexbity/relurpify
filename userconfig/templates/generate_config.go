package templates

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/userconfig/securefile"
	"codeburg.org/lexbit/relurpify/userconfig/templates/embedfs"
)

// GenerateConfig writes the embedded template trees to the given output
// directory, mirroring their layout 1:1: the "workspace" subtree becomes the
// config root and the "prompts" subtree lands in prompts/ (the default prompt
// corpus ships inside relurpify_cfg so a materialized workspace resolves its
// system prompts without a source checkout). It is the single producer of the
// checked-in relurpify_cfg/ tree.
// Source: embedfs.DefaultFS() subtrees "workspace" and "prompts".
func GenerateConfig(output string) error {
	output = filepath.Clean(output)
	if output == "." || output == string(filepath.Separator) {
		return fmt.Errorf("refusing to generate config into %q", output)
	}
	efs := embedfs.DefaultFS()
	if err := os.RemoveAll(output); err != nil {
		return fmt.Errorf("clean output dir %q: %w", output, err)
	}
	if err := securefile.MkdirAllSecure(output); err != nil {
		return fmt.Errorf("create output dir %q: %w", output, err)
	}
	// The workspace tree maps onto the config root; the prompts tree keeps
	// its prompts/ prefix so the corpus lands in <output>/prompts/.
	writeTree := func(root string, stripRoot bool) error {
		return fs.WalkDir(efs, root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == root {
				return nil
			}
			rel := path
			if stripRoot {
				rel = strings.TrimPrefix(path, root+"/")
			}
			target := filepath.Join(output, rel)
			if d.IsDir() {
				return securefile.MkdirAllSecure(target)
			}
			data, err := fs.ReadFile(efs, path)
			if err != nil {
				return err
			}
			if err := securefile.MkdirAllSecure(filepath.Dir(target)); err != nil {
				return err
			}
			return securefile.WriteFileSecure(target, data)
		})
	}
	if err := writeTree("workspace", true); err != nil {
		return err
	}
	return writeTree("prompts", false)
}
