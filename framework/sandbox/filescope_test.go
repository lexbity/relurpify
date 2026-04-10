package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestFileScopePolicyCheck(t *testing.T) {
	dir := t.TempDir()
	protected := filepath.Join(dir, "relurpify_cfg", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(protected), 0o755))
	require.NoError(t, os.WriteFile(protected, []byte("config"), 0o644))

	policy := NewFileScopePolicy(dir, []string{protected})
	err := policy.Check(core.FileSystemWrite, protected)
	require.ErrorIs(t, err, ErrFileScopeProtectedPath)

	err = policy.Check(core.FileSystemWrite, filepath.Join(dir, "..", "escape.txt"))
	require.ErrorIs(t, err, ErrFileScopeOutsideWorkspace)
}
