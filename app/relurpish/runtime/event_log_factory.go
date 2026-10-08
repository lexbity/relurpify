package runtime

import (
	"codeburg.org/lexbit/relurpify/telemetry/event"
)

// openRuntimeEventLogFactory is the composition-root assignment for
// session.WorkspaceConfig.EventLogFactory (FR-4): the durable causal record
// is Badger-backed at the workspace's events.db path. Opening failures are
// surfaced so OpenWorkspace can fail closed to JSONL-only telemetry (NFR-4).
func openRuntimeEventLogFactory(path string) (event.Log, error) {
	return event.NewBadgerLog(path)
}
