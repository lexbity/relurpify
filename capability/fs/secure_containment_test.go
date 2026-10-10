package fs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveWithinBase_AllowsNestedLeaf(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "nested")
	require.NoError(t, MkdirAllSecure(parent))

	resolved, err := ResolveWithinBase(base, filepath.Join("nested", "new.txt"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(base, "nested", "new.txt"), resolved)
}

func TestResolveWithinBase_RejectsOutsidePath(t *testing.T) {
	base := t.TempDir()

	_, err := ResolveWithinBase(base, "../etc/passwd")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrPathEscapesBase)
}

func TestResolveWithinBase_RejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	require.NoError(t, WriteFileSecure(outsideFile, []byte("secret")))

	link := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(outside, link))

	_, err := ResolveWithinBase(base, filepath.Join("link", "secret.txt"))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrPathEscapesBase)
}
