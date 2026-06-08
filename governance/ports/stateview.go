package ports

// StateView is the governance-owned view of the execution state
// (contextdata.Envelope). Governance passes this through to agents
// without needing to read envelope fields directly.
type StateView interface {
	// TaskID returns the task identifier for this execution.
	TaskID() string
	// SessionID returns the session identifier for this execution.
	SessionID() string
}

// SearchScope is the governance-owned interface for glob matching.
type SearchScope interface {
	// MatchGlob checks whether a path matches a glob pattern.
	MatchGlob(pattern, target string) bool
}
