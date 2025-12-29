package agenttest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFallbackManifestPath(t *testing.T) {
	workspace := t.TempDir()
	manifest := filepath.Join(workspace, "relurpify_cfg", "agent.manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(manifest, []byte("test"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got := fallbackManifestPath(filepath.Join(workspace, "testsuite", "agent.manifest.yaml"), workspace)
	if got != manifest {
		t.Fatalf("expected %s, got %s", manifest, got)
	}
}
