package search

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrepTool_RejectsOutsidePath(t *testing.T) {
	tool := &GrepTool{BasePath: t.TempDir()}
	_, err := tool.Execute(context.Background(), map[string]any{
		"directory": "../etc",
		"pattern":   "TODO",
	})
	require.Error(t, err)
}

func TestGrepTool_RejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("TODO"), 0o600))

	link := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(outside, link))

	tool := &GrepTool{BasePath: base}
	_, err := tool.Execute(context.Background(), map[string]any{
		"directory": "link",
		"pattern":   "TODO",
	})
	require.Error(t, err)
}
