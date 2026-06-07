package graphdb

import "time"

type SyncMode string

const (
	SyncAlways   SyncMode = "always"
	SyncInterval SyncMode = "interval"
	SyncOnFlush  SyncMode = "flush"
)

// Options configures engine persistence and maintenance behavior.
type Options struct {
	DataDir                  string
	AOFFileName              string
	SnapshotFileName         string
	SnapshotOnClose          bool
	SyncMode                 SyncMode
	SyncInterval             time.Duration
	AutoSaveInterval         time.Duration
	AutoSaveThreshold        int64
	AOFRewriteThresholdBytes int64
	MaintenanceInterval      time.Duration
}

// DefaultOptions returns a standard graphdb configuration.
func DefaultOptions(dataDir string) Options {
	return Options{
		DataDir:                  dataDir,
		AOFFileName:              "graphdb.aof",
		SnapshotFileName:         "graphdb.snapshot",
		SnapshotOnClose:          false,
		SyncMode:                 SyncAlways,
		SyncInterval:             250 * time.Millisecond,
		AutoSaveInterval:         time.Minute,
		AutoSaveThreshold:        1000,
		AOFRewriteThresholdBytes: 8 << 20,
		MaintenanceInterval:      10 * time.Second,
	}
}
