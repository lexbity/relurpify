package artifactstore

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPutOpenRoundTrip(t *testing.T) {
	workspace := t.TempDir()
	store, err := NewDiskStore(workspace, 0)
	require.NoError(t, err)
	defer store.Close()

	content := "hello world"
	ref, err := store.Put(context.Background(), "text", map[string]string{"session": "test-session"}, strings.NewReader(content))
	require.NoError(t, err)
	require.NotEmpty(t, ref)
	require.Equal(t, "test-session", ref.Session())

	rc, meta, err := store.Open(context.Background(), ref)
	require.NoError(t, err)
	defer rc.Close()

	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Equal(t, content, string(data))
	require.Equal(t, "text", meta.Kind)
	require.Equal(t, int64(len(content)), meta.Size)
}

func TestRefStability(t *testing.T) {
	workspace := t.TempDir()
	store, err := NewDiskStore(workspace, 0)
	require.NoError(t, err)
	defer store.Close()

	ref, err := store.Put(context.Background(), "test", nil, strings.NewReader("data"))
	require.NoError(t, err)

	// Ref format: artifact://<session>/<id>
	refStr := string(ref)
	require.True(t, strings.HasPrefix(refStr, "artifact://"), "ref should start with artifact://")
	parts := strings.SplitN(refStr[12:], "/", 2)
	require.Len(t, parts, 2, "ref should have session/id parts")
	require.NotEmpty(t, parts[0], "session should not be empty")
	require.NotEmpty(t, parts[1], "id should not be empty")
}

func TestLargeStreaming(t *testing.T) {
	workspace := t.TempDir()
	store, err := NewDiskStore(workspace, 0)
	require.NoError(t, err)
	defer store.Close()

	// Stream 5 MiB without OOM.
	size := 5 * 1024 * 1024
	content := make([]byte, size)
	for i := range content {
		content[i] = byte(i % 256)
	}

	ref, err := store.Put(context.Background(), "binary", nil, bytes.NewReader(content))
	require.NoError(t, err)

	rc, meta, err := store.Open(context.Background(), ref)
	require.NoError(t, err)
	defer rc.Close()

	read, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Equal(t, size, len(read))
	require.Equal(t, int64(size), meta.Size)
}

func TestPutNilReader(t *testing.T) {
	workspace := t.TempDir()
	store, err := NewDiskStore(workspace, 0)
	require.NoError(t, err)
	defer store.Close()

	_, err = store.Put(context.Background(), "test", nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "reader required")
}

func TestOpenNotFound(t *testing.T) {
	workspace := t.TempDir()
	store, err := NewDiskStore(workspace, 0)
	require.NoError(t, err)
	defer store.Close()

	_, _, err = store.Open(context.Background(), Ref("artifact://nonexistent/1234"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestDefaultSession(t *testing.T) {
	workspace := t.TempDir()
	store, err := NewDiskStore(workspace, 0)
	require.NoError(t, err)
	defer store.Close()

	ref, err := store.Put(context.Background(), "test", nil, strings.NewReader("data"))
	require.NoError(t, err)
	require.Equal(t, "default", ref.Session())
}

func TestMultipleArtifacts(t *testing.T) {
	workspace := t.TempDir()
	store, err := NewDiskStore(workspace, 0)
	require.NoError(t, err)
	defer store.Close()

	ref1, err := store.Put(context.Background(), "a", map[string]string{"session": "s1"}, strings.NewReader("data1"))
	require.NoError(t, err)

	ref2, err := store.Put(context.Background(), "b", map[string]string{"session": "s1"}, strings.NewReader("data2"))
	require.NoError(t, err)

	ref3, err := store.Put(context.Background(), "c", map[string]string{"session": "s2"}, strings.NewReader("data3"))
	require.NoError(t, err)

	// Verify each.
	for _, tc := range []struct {
		ref  Ref
		want string
		sess string
	}{
		{ref1, "data1", "s1"},
		{ref2, "data2", "s1"},
		{ref3, "data3", "s2"},
	} {
		rc, _, err := store.Open(context.Background(), tc.ref)
		require.NoError(t, err)
		data, _ := io.ReadAll(rc)
		rc.Close()
		require.Equal(t, tc.want, string(data))
		require.Equal(t, tc.sess, tc.ref.Session())
	}
}

func TestDiskStoreDirectoryStructure(t *testing.T) {
	workspace := t.TempDir()
	store, err := NewDiskStore(workspace, 0)
	require.NoError(t, err)
	defer store.Close()

	ref, err := store.Put(context.Background(), "test", map[string]string{"session": "mysession"}, strings.NewReader("content"))
	require.NoError(t, err)

	// Verify files on disk.
	expectedDir := filepath.Join(workspace, ".relurpify_state", "artifacts", "mysession")
	_, err = os.Stat(expectedDir)
	require.NoError(t, err, "session dir should exist")

	// Ref is artifact://mysession/<id>
	id := string(ref[12+9:]) // skip "artifact://mysession/"
	dataPath := filepath.Join(expectedDir, id)
	metaPath := dataPath + ".meta"

	_, err = os.Stat(dataPath)
	require.NoError(t, err, "data file should exist")
	_, err = os.Stat(metaPath)
	require.NoError(t, err, "meta file should exist")
}

func TestScanOnBoot(t *testing.T) {
	workspace := t.TempDir()

	// Create first store instance and write artifacts.
	store1, err := NewDiskStore(workspace, 0)
	require.NoError(t, err)

	_, err = store1.Put(context.Background(), "text", map[string]string{"session": "session-1"}, strings.NewReader("data1"))
	require.NoError(t, err)

	_, err = store1.Put(context.Background(), "text", map[string]string{"session": "session-2"}, strings.NewReader("data22"))
	require.NoError(t, err)

	err = store1.Close()
	require.NoError(t, err)

	// Recreate store using the same workspace directory.
	store2, err := NewDiskStore(workspace, 0)
	require.NoError(t, err)
	defer store2.Close()

	// Verify that the total size and session sizes are parsed on boot.
	require.Greater(t, store2.TotalBytes(), int64(0))
	require.Contains(t, store2.sessions, "session-1")
	require.Contains(t, store2.sessions, "session-2")
	require.Greater(t, store2.sessions["session-1"].Size, int64(5))
	require.Greater(t, store2.sessions["session-2"].Size, int64(6))

	// Total bytes should match the sum of sessions.
	var expectedTotal int64
	for _, state := range store2.sessions {
		expectedTotal += state.Size
	}
	require.Equal(t, expectedTotal, store2.TotalBytes())
}
