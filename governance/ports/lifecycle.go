package ports

// DelegationEntry is the governance-owned view of a delegation record.
type DelegationEntry struct {
	DelegationID string
	WorkflowID   string
	RunID        string
	State        string
}

// DelegationTransitionEntry records a state transition in a delegation.
type DelegationTransitionEntry struct {
	DelegationID string
	FromState    string
	ToState      string
	Reason       string
}

// WorkflowArtifactRecord is the governance-owned view of a workflow artifact.
type WorkflowArtifactRecord struct {
	ArtifactID  string
	WorkflowID  string
	RunID       string
	ContentType string
	StorageKind string
	SummaryText string
	Metadata    map[string]any
}

// LifecycleView is the governance-owned interface for persisting
// delegation lifecycle data. execution/agentlifecycle implements it.
type LifecycleView interface {
	PersistDelegation(entry DelegationEntry) error
	PersistDelegationTransition(transition DelegationTransitionEntry) error
	StoreArtifact(record WorkflowArtifactRecord) error
}
