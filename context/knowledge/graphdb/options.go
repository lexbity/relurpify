package graphdb

import "time"

// BackendKind selects the durable storage implementation.
type BackendKind string

const (
	// BackendBadger uses Badger as the durable store (default for new stores).
	BackendBadger BackendKind = "badger"
	// BackendAOF uses the classic AOF+snapshot file format.
	BackendAOF BackendKind = "aof"
)

type SyncMode string

const (
	SyncAlways   SyncMode = "always"
	SyncInterval SyncMode = "interval"
	SyncOnFlush  SyncMode = "flush"
)

// Options configures engine persistence and maintenance behavior.
type Options struct {
	Backend  BackendKind
	BadgerDir string // data directory for Badger (defaults to DataDir if empty)
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

	// Observer receives structured events emitted by the engine. When nil
	// (the default) events are silently dropped.
	Observer EventObserver
}

// DefaultOptions returns a standard graphdb configuration with Badger
// as the default durable backend.
func DefaultOptions(dataDir string) Options {
	return Options{
		Backend:                  BackendBadger,
		BadgerDir:                dataDir,
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

// DefaultAOFOptions returns a standard graphdb configuration with the
// classic AOF backend. This should be used only for backward‑compatible
// migration code and AOF‑specific tests.
func DefaultAOFOptions(dataDir string) Options {
	opts := DefaultOptions(dataDir)
	opts.Backend = BackendAOF
	return opts
}
