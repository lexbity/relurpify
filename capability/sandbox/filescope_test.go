package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/governance/permissions"
)

func TestFileScopePolicyCheck(t *testing.T) {
	dir := t.TempDir()
	protected := filepath.Join(dir, "relurpify_cfg", "manifest.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(protected), 0o700))
	require.NoError(t, os.WriteFile(protected, []byte("config"), 0o600))

	policy := NewFileScopePolicy(dir, []string{protected})
	err := policy.Check(permissions.FileSystemWrite, protected)
	require.ErrorIs(t, err, ErrFileScopeProtectedPath)

	err = policy.Check(permissions.FileSystemWrite, filepath.Join(dir, "..", "escape.txt"))
	require.ErrorIs(t, err, ErrFileScopeOutsideWorkspace)
}
