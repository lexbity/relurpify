package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSQLiteFMPExportStoreRoundTrip(t *testing.T) {
	t.Parallel()

	store, err := NewSQLiteFMPExportStore(filepath.Join(t.TempDir(), "fmp_exports.db"))
	require.NoError(t, err)
	defer store.Close()

	require.NoError(t, store.SetTenantExportEnabled(context.Background(), "tenant-1", "exp.run", false))
	enabled, configured, err := store.IsExportEnabled(context.Background(), "tenant-1", "exp.run")
	require.NoError(t, err)
	require.True(t, configured)
	require.False(t, enabled)

	list, err := store.ListTenantExports(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "exp.run", list[0].ExportName)
	require.False(t, list[0].Enabled)
}
