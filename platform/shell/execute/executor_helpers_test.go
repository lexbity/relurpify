package execute

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecutorHelperBranches(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "keep.txt"), []byte("keep"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(src, "ignore.bak"), []byte("skip"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(src, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, ".git", "config"), []byte("skip"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(src, "target"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "target", "artifact"), []byte("skip"), 0o644))

	require.Equal(t, filepath.Clean("nested"), resolvePath("", "nested"))
	require.Equal(t, filepath.Join(base, "nested"), resolvePath(base, "nested"))
	abs := filepath.Join(base, "absolute")
	require.Equal(t, abs, resolvePath(base, abs))

	manifest := filepath.Join(base, "Cargo.toml")
	require.Equal(t, []string{"--manifest-path", manifest}, withManifestPath(nil, manifest))
	require.Equal(t, []string{"test", "--manifest-path", manifest}, withManifestPath([]string{"test"}, manifest))
	require.Equal(t, []string{"--manifest-path", manifest, "--verbose"}, withManifestPath([]string{"--verbose"}, manifest))
	require.Equal(t, []string{"test", "--manifest-path", manifest}, withManifestPath([]string{"test", "--manifest-path", manifest}, manifest))

	dst := filepath.Join(t.TempDir(), "mirror")
	require.NoError(t, copyDir(src, dst))
	_, err := os.Stat(filepath.Join(dst, "keep.txt"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dst, ".git"))
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(dst, "target"))
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(dst, "ignore.bak"))
	require.Error(t, err)

	require.Error(t, copyDir(filepath.Join(base, "missing"), filepath.Join(base, "dst")))
	require.Error(t, copyFile(filepath.Join(base, "missing.txt"), filepath.Join(base, "out.txt"), 0o644))

	isolated, err := isolateCargoWorkdir(src)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(filepath.Dir(isolated)))
}
