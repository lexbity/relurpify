package tooltest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAllToolTests loads and runs every .tooltest.yaml file found in the
// workspace's relurpify_cfg/tooltests directory. Each file is a self-contained
// tool invocation test that exercises the real subprocess executor path.
func TestAllToolTests(t *testing.T) {
	workspace := workspaceRoot(t)
	dir := DefaultToolTestDir(workspace)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("tooltests directory not found")
			return
		}
		t.Fatalf("read tooltests dir: %v", err)
	}

	count := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".yaml" || !strings.Contains(entry.Name(), ".tooltest.") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		tc, err := LoadToolTest(path)
		if err != nil {
			t.Errorf("load %s: %v", path, err)
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			RunToolTest(t, workspace, tc)
		})
		count++
	}

	if count == 0 {
		t.Log("no tooltest files found")
	}
	if count < 10 {
		t.Logf("warning: only %d tooltest files (want ≥10)", count)
	}
}
