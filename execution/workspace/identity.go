package workspace

import (
	"fmt"
	"path/filepath"
)

// StateDirName is the canonical name for the runtime state directory
// relative to the workspace root.
const StateDirName = ".relurpify_state"

// Identity holds normalized workspace identity information derived from
// a workspace root path. Root is always an absolute, cleaned path.
type Identity struct {
	Root string
}

// New validates and normalizes a workspace root path, returning an Identity
// with an absolute, cleaned Root. Returns an error if root is empty.
func New(root string) (Identity, error) {
	if root == "" {
		return Identity{}, fmt.Errorf("workspace root required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Identity{}, fmt.Errorf("resolve workspace root: %w", err)
	}
	return Identity{Root: filepath.Clean(abs)}, nil
}

// StateDir returns the default runtime state directory under the workspace root.
func (id Identity) StateDir() string {
	return filepath.Join(id.Root, StateDirName)
}

// LogDir returns the default log directory under the state directory.
func (id Identity) LogDir() string {
	return filepath.Join(id.StateDir(), "logs")
}

// LogPath returns a path to a named log file under the state log directory.
func (id Identity) LogPath(name string) string {
	return filepath.Join(id.LogDir(), name)
}

// TelemetryDir returns the default telemetry directory under the state directory.
func (id Identity) TelemetryDir() string {
	return filepath.Join(id.StateDir(), "telemetry")
}

// TelemetryPath returns a path to a named telemetry file under the state telemetry directory.
func (id Identity) TelemetryPath(name string) string {
	return filepath.Join(id.TelemetryDir(), name)
}

// EventsFile returns the default events database path under the state directory.
func (id Identity) EventsFile() string {
	return filepath.Join(id.StateDir(), "events.db")
}

// MemoryDir returns the default memory/working store directory under the state directory.
func (id Identity) MemoryDir() string {
	return filepath.Join(id.StateDir(), "memory")
}

// StateDir returns the default runtime state directory for the given workspace root.
// This is a package-level convenience function when no Identity constructor
// validation is needed.
func StateDir(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, StateDirName)
}
