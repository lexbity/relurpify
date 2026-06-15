package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/templates"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: driftcheck <output-dir>")
		os.Exit(1)
	}
	outDir := os.Args[1]

	efs := templatesembed.DefaultFS()
	err := fs.WalkDir(efs, "workspace", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, "workspace/")
		if rel == "" {
			return nil
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(outDir, "workspace", rel), 0755)
		}
		data, err := fs.ReadFile(efs, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(outDir, "workspace", rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0644)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "driftcheck: materialize workspace templates: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("materialised embedded templates to %s/workspace\n", outDir)
}
