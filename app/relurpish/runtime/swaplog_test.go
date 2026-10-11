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

// distinctWriter is a different concrete io.Writer type, mirroring the
// production swap: the process starts on *os.File (stderr) and the first
// runtime repoint stores the workspace's rotating writer.
type distinctWriter struct {
	buf []byte
}

func (d *distinctWriter) Write(p []byte) (int, error) {
	d.buf = append(d.buf, p...)
	return len(p), nil
}

// TestStandardLogWriterAcceptsMixedConcreteTypes proves the swap survives
// targets of different concrete types. The pre-fix implementation stored the
// raw writer in an atomic.Value, whose second store of a different concrete
// type panics ("sync/atomic: store of inconsistently typed value") — every
// process that reached RepointStdlibLog died at boot.
func TestStandardLogWriterAcceptsMixedConcreteTypes(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "boot.log"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })

	w := NewStandardLogWriter(file) // *os.File, as the package initializer stores
	_, err = w.Write([]byte("on-file\n"))
	require.NoError(t, err)

	distinct := &distinctWriter{}
	require.NotPanics(t, func() { w.Set(distinct) }, "Set must accept a different concrete writer type")
	_, err = w.Write([]byte("on-distinct\n"))
	require.NoError(t, err)
	require.Equal(t, "on-distinct\n", string(distinct.buf))
}

// TestStandardLogWriterConcurrentSwapAndWrite races Set against Write to pin
// the swap's race freedom under the production pattern (a boot repoint while
// early boot lines are still being written).
func TestStandardLogWriterConcurrentSwapAndWrite(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "race.log"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })

	w := NewStandardLogWriter(file)
	distinct := &distinctWriter{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			_, _ = w.Write([]byte("line\n"))
		}
	}()
	for i := 0; i < 100; i++ {
		w.Set(distinct)
		w.Set(file)
	}
	<-done
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
