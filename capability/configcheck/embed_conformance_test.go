package configcheck

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/templates"
)

func TestEmbeddedToolsPassConfigcheck(t *testing.T) {
	tmpDir := t.TempDir()
	if err := templates.GenerateConfig(tmpDir); err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}
	toolsDir := filepath.Join(tmpDir, "tools")
	manifests, err := config.LoadToolManifests(toolsDir)
	if err != nil {
		t.Fatalf("LoadToolManifests: %v", err)
	}
	if len(manifests) == 0 {
		t.Fatal("no tool manifests loaded from embedded tree")
	}
	results := CheckAllManifests(manifests)
	if len(results) > 0 {
		for name, issues := range results {
			for _, issue := range issues {
				t.Errorf("embedded tool %q: %s", name, issue)
			}
		}
		t.Fatalf("%d embedded tools failed configcheck", len(results))
	}
}

func TestOnDiskToolsPassConfigcheck(t *testing.T) {
	toolsDir := filepath.Join("..", "..", "relurpify_cfg", "tools")
	if _, err := os.Stat(toolsDir); err != nil {
		t.Skip("relurpify_cfg/tools not found, skipping")
	}
	manifests, err := config.LoadToolManifests(toolsDir)
	if err != nil {
		t.Fatalf("LoadToolManifests: %v", err)
	}
	if len(manifests) == 0 {
		t.Fatal("no tool manifests loaded from on-disk tree")
	}
	results := CheckAllManifests(manifests)
	if len(results) > 0 {
		for name, issues := range results {
			for _, issue := range issues {
				t.Errorf("on-disk tool %q: %s", name, issue)
			}
		}
		t.Fatalf("%d on-disk tools failed configcheck", len(results))
	}
}
