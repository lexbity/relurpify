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
	// BadgerDir is the data directory for the Badger store. Defaults to
	// DataDir when empty.
	BadgerDir string

	// DataDir is the top-level state directory.
	DataDir string

	// AOFFileName and SnapshotFileName are retained only for reading
	// legacy AOF stores during migration. They are not used by the
	// Badger backend.
	AOFFileName              string
	SnapshotFileName         string
	SnapshotOnClose          bool
	SyncMode                 SyncMode
	SyncInterval             time.Duration
	AutoSaveInterval         time.Duration
	AutoSaveThreshold        int64
	AOFRewriteThresholdBytes int64
	MaintenanceInterval      time.Duration

	// LRUCapacity controls the maximum number of nodes kept in the
	// in-memory working set. When 0 (default), all nodes are loaded into
	// RAM (legacy behaviour). When > 0, the engine serves reads via an
	// LRU over Badger (NFR-9). The capacity should be derived from the
	// sandbox RAM budget.
	LRUCapacity int

	// Observer receives structured events emitted by the engine. When nil
	// (the default) events are silently dropped.
	Observer EventObserver
}

// DefaultOptions returns a standard graphdb configuration with Badger
// as the durable backend.
func DefaultOptions(dataDir string) Options {
	return Options{
		BadgerDir:                dataDir,
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
