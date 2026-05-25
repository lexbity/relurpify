package cfgload

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollectPhaseOneInventorySynthetic(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "framework/agentenv/workspace.go", `package agentenv

import "os"

func read(path string) string {
	return os.ReadFile(path)
}
`)
	writeFixture(t, root, "framework/templates/resolver.go", `package templates

import (
	"os"
	"path/filepath"
)

func shared() string {
	if v := os.Getenv("X"); v != "" {
		return v
	}
	return filepath.Abs(".")
}
`)

	inv, err := CollectInventory(root, []string{
		"framework/agentenv/workspace.go",
		"framework/templates/resolver.go",
	})
	require.NoError(t, err)
	require.Len(t, inv.Findings, 3)
	require.Equal(t, FindingKindConfigRead, inv.Findings[0].Kind)
	require.Equal(t, FindingKindEnvLookup, inv.Findings[1].Kind)
	require.Equal(t, FindingKindPathResolve, inv.Findings[2].Kind)
}

func TestPhaseOneInventoryMatchesSnapshot(t *testing.T) {
	root := repoRoot(t)
	inv, err := CollectPhaseOneInventory(root)
	require.NoError(t, err)

	got := inv.Render()
	wantPath := filepath.Join(root, "framework", "cfgload", "testdata", "phase1_inventory.golden")
	wantBytes, err := os.ReadFile(wantPath)
	require.NoError(t, err)
	require.Equal(t, string(wantBytes), got)
}

func TestPhaseOneInventoryAuditStrictFiltersCfgloadOnly(t *testing.T) {
	inv := Inventory{
		Findings: []Finding{
			{File: "framework/cfgload/inventory.go"},
			{File: "app/relurpish/runtime/config.go"},
		},
	}
	err := inv.AuditStrict()
	require.Error(t, err)
	require.Contains(t, err.Error(), "ambient config findings")
}

func writeFixture(t *testing.T, root, rel, contents string) {
	t.Helper()
	path := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o644))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}
