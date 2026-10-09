package runtime

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/execution/session"
)

// TestStandardLogWriterSwapsTargetWithoutStaleWrites proves the swappable
// writer retargets atomically and never writes into the replaced target after
// a Set — the exact property a workspace reload needs so the global logger
// cannot land a line in the old runtime's closed log file.
func TestStandardLogWriterSwapsTargetWithoutStaleWrites(t *testing.T) {
	oldPath := filepath.Join(t.TempDir(), "old.log")
	newPath := filepath.Join(t.TempDir(), "new.log")
	oldFile, err := os.Create(oldPath)
	require.NoError(t, err)
	newFile, err := os.Create(newPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = oldFile.Close() })
	t.Cleanup(func() { _ = newFile.Close() })

	w := NewStandardLogWriter(oldFile)
	_, err = w.Write([]byte("before-swap\n"))
	require.NoError(t, err)

	w.Set(newFile)
	_, err = w.Write([]byte("after-swap\n"))
	require.NoError(t, err)

	// The old target is closed and detached; the new target received the line.
	require.NoError(t, oldFile.Close())

	oldRaw, err := os.ReadFile(oldPath)
	require.NoError(t, err)
	require.Equal(t, "before-swap\n", string(oldRaw))

	newRaw, err := os.ReadFile(newPath)
	require.NoError(t, err)
	require.Equal(t, "after-swap\n", string(newRaw))
}

// TestRepointStdlibLogRedirectsGlobalLogger proves the reload seam: after a
// workspace reload repoints the process swappable writer at the new runtime's
// log file, stdlib log lines land in the new file and the old file (now
// closed) receives nothing more.
func TestRepointStdlibLogRedirectsGlobalLogger(t *testing.T) {
	prevOutput := log.Writer()
	t.Cleanup(func() {
		log.SetOutput(prevOutput)
		StdlibLogWriter().Set(os.Stderr)
	})

	oldPath := filepath.Join(t.TempDir(), "old.log")
	newPath := filepath.Join(t.TempDir(), "new.log")
	oldFile, err := os.Create(oldPath)
	require.NoError(t, err)
	newFile, err := os.Create(newPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = newFile.Close() })

	log.SetOutput(StdlibLogWriter())
	RepointStdlibLog(&Runtime{Workspace: &session.Workspace{Logger: log.New(oldFile, "", 0)}})
	log.Print("line-before-reload")

	// The ReloadRuntimeForWorkspace seam: build the replacement, repoint at it,
	// then close the old runtime's log file (its Workspace.Close does this).
	RepointStdlibLog(&Runtime{Workspace: &session.Workspace{Logger: log.New(newFile, "", 0)}})
	require.NoError(t, oldFile.Close())
	log.Print("line-after-reload")

	require.NoError(t, newFile.Close())

	oldRaw, err := os.ReadFile(oldPath)
	require.NoError(t, err)
	require.Contains(t, string(oldRaw), "line-before-reload")
	require.NotContains(t, string(oldRaw), "line-after-reload")

	newRaw, err := os.ReadFile(newPath)
	require.NoError(t, err)
	require.Contains(t, string(newRaw), "line-after-reload")
}
