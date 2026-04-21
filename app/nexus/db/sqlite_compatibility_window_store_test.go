package db

import (
	"context"
	"path/filepath"
	"testing"

	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
	"github.com/stretchr/testify/require"
)

func TestSQLiteCompatibilityWindowStoreRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "compatibility.db")
	store, err := NewSQLiteCompatibilityWindowStore(path)
	require.NoError(t, err)
	defer store.Close()

	_, err = NewSQLiteCompatibilityWindowStore("   ")
	require.Error(t, err)

	_, ok, err := store.GetWindow(ctx, "   ")
	require.Error(t, err)
	require.False(t, ok)

	require.Error(t, store.UpsertWindow(ctx, fwfmp.CompatibilityWindow{}))
	require.Error(t, store.DeleteWindow(ctx, "   "))

	window := fwfmp.CompatibilityWindow{
		ContextClass:      "workflow-runtime",
		MinSchemaVersion:  "1.2.0",
		MaxSchemaVersion:  "1.4.0",
		MinRuntimeVersion: "2.0.0",
		MaxRuntimeVersion: "2.5.0",
	}
	require.NoError(t, store.UpsertWindow(ctx, window))

	got, ok, err := store.GetWindow(ctx, "workflow-runtime")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, window.ContextClass, got.ContextClass)
	require.Equal(t, window.MinSchemaVersion, got.MinSchemaVersion)
	require.Equal(t, window.MaxRuntimeVersion, got.MaxRuntimeVersion)

	window.MinRuntimeVersion = "2.1.0"
	require.NoError(t, store.UpsertWindow(ctx, window))
	got, ok, err = store.GetWindow(ctx, "workflow-runtime")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "2.1.0", got.MinRuntimeVersion)

	windows, err := store.ListWindows(ctx)
	require.NoError(t, err)
	require.Len(t, windows, 1)
	require.Equal(t, "workflow-runtime", windows[0].ContextClass)

	require.NoError(t, store.DeleteWindow(ctx, "workflow-runtime"))
	got, ok, err = store.GetWindow(ctx, "workflow-runtime")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, got)

	require.NoError(t, store.Close())
	require.NoError(t, (*SQLiteCompatibilityWindowStore)(nil).Close())
}
