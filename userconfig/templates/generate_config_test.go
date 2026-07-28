package templates

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/userconfig/templates/embedfs"
)

func TestGenerateConfig_MirrorsEmbedLayout(t *testing.T) {
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "out")
	if err := GenerateConfig(outDir); err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}
	efs := embedfs.DefaultFS()
	if err := fs.WalkDir(efs, "workspace", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "workspace" {
			return nil
		}
		rel := strings.TrimPrefix(path, "workspace/")
		target := filepath.Join(outDir, rel)
		if d.IsDir() {
			info, err := os.Stat(target)
			if err != nil {
				return err
			}
			if !info.IsDir() {
				return fmt.Errorf("expected dir %q, got file", target)
			}
			return nil
		}
		embedData, err := fs.ReadFile(efs, path)
		if err != nil {
			return err
		}
		diskData, err := os.ReadFile(filepath.Clean(target)) //nolint:gosec // test reads generated output under temp dir to verify byte-equality
		if err != nil {
			return err
		}
		if len(embedData) != len(diskData) {
			return fmt.Errorf("file %q: embed size %d != disk size %d", rel, len(embedData), len(diskData))
		}
		for i := range embedData {
			if embedData[i] != diskData[i] {
				return fmt.Errorf("file %q: content differs at byte %d", rel, i)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateConfig_RefusesUnsafeOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"dot", "."},
		{"root", string(filepath.Separator)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := GenerateConfig(tt.output)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestGenerateConfig_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	out1 := filepath.Join(tmpDir, "out1")
	out2 := filepath.Join(tmpDir, "out2")
	if err := GenerateConfig(out1); err != nil {
		t.Fatalf("first GenerateConfig: %v", err)
	}
	if err := GenerateConfig(out2); err != nil {
		t.Fatalf("second GenerateConfig: %v", err)
	}
	if err := filepath.WalkDir(out1, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(out1, path)
		if err != nil {
			return err
		}
		otherPath := filepath.Join(out2, rel)
		data1, err := os.ReadFile(filepath.Clean(path)) //nolint:gosec // test reads generated output under temp dir to verify idempotency
		if err != nil {
			return err
		}
		data2, err := os.ReadFile(filepath.Clean(otherPath)) //nolint:gosec // test reads generated output under temp dir to verify idempotency
		if err != nil {
			return err
		}
		if len(data1) != len(data2) {
			return fmt.Errorf("output %q: size %d != %d", rel, len(data1), len(data2))
		}
		for i := range data1 {
			if data1[i] != data2[i] {
				return fmt.Errorf("output %q: byte %d differs", rel, i)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestGenerateConfig_StaleOutputCleaned verifies that files not in the embed
// tree are removed from the output on regeneration.
func TestGenerateConfig_StaleOutputCleaned(t *testing.T) {
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "out")
	if err := GenerateConfig(outDir); err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}
	stale := filepath.Join(outDir, "stale_file.yaml")
	if err := os.WriteFile(stale, []byte("stale"), 0644); err != nil { //nolint:gosec // test fixture writes a stale file under temp dir to verify it is removed on regeneration
		t.Fatal(err)
	}
	if err := GenerateConfig(outDir); err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Fatal("stale file should have been removed on regeneration")
	}
}
