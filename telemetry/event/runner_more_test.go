package event

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunnerRestoreAndPartitionDefaults(t *testing.T) {
	log := &memoryLog{}
	_, err := log.Append(context.Background(), "local", []FrameworkEvent{
		{Timestamp: time.Now().UTC(), Type: EventSystemStarted, Partition: "local"},
	})
	require.NoError(t, err)
	mat := &recordingMaterializer{}
	runner := &Runner{
		Log:              log,
		Materializers:    []Materializer{mat},
		SnapshotInterval: 1,
	}
	require.Equal(t, "local", partitionOrDefault(""))
	require.NoError(t, runner.RestoreAndRunOnce(context.Background()))
	require.Len(t, mat.applied, 1)
	require.NotEmpty(t, log.snapshots)
}

func TestRunnerNilNoops(t *testing.T) {
	require.NoError(t, (*Runner)(nil).RunOnce(context.Background()))
	require.NoError(t, (*Runner)(nil).RestoreAndRunOnce(context.Background()))
}
