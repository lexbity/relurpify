package ports

// State is the minimal interface a capability handler needs from the
// context execution envelope. capability/ports owns this interface so
// that capability handlers never import context directly; context's
// Envelope satisfies it.
//
// Implementations must be safe for concurrent read/write.
type State interface {
	// GetWorkingValue retrieves a value from working memory.
	GetWorkingValue(key string) (any, bool)

	// SetWorkingValue stores a value in working memory.
	SetWorkingValue(key string, value any)

	// DeleteWorkingValue removes a value from working memory.
	DeleteWorkingValue(key string)

	// ClearWorkingData removes all working memory values.
	ClearWorkingData()

	// WorkingMemoryKeys returns all keys currently in working memory.
	WorkingMemoryKeys() []string

	// Snapshot returns a point-in-time copy of all working memory.
	Snapshot() map[string]any

	// TaskID returns the task identifier for this execution.
	TaskID() string

	// SessionID returns the session identifier for this execution.
	SessionID() string
}
