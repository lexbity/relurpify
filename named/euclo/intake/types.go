package intake

import "codeburg.org/lexbit/relurpify/named/euclo/families"

// IntentClassification holds the result of tier-1 and tier-2 classification.
// To be fully implemented in Phase 4 and 6.
type IntentClassification struct {
	WinningFamily        string
	FamilyCandidates     []families.FamilyCandidate
	Confidence           float64
	Ambiguous            bool
	Signals              []ClassificationSignal
	NegativeConstraints  []string
	CapabilitySequence   []string
	CapabilityOperator   string
	ClassificationSource string
	MixedIntent          bool
	EditPermitted        bool
	RequiresVerification bool
	Scope                string
	RiskLevel            string
	ReasonCodes          []string
}

// TaskEnvelope is the canonical representation of a user request for Euclo.
// It contains the normalized instruction with all hints extracted and parsed.
type TaskEnvelope struct {
	// Core identity
	TaskID      string
	SessionID   string
	Instruction string
	TaskType    string // Defaults to "analysis"

	// Parsed hints (extracted from message)
	ContextHint     string   // e.g., "context-hint: typescript-react"
	SessionHint     string   // e.g., "session-hint: continue-refactoring"
	FollowUpHint    string   // e.g., "follow-up: implement-tests"
	AgentModeHint   string   // e.g., "mode: architect"
	WorkspaceScopes []string // e.g., "workspace-scope: backend, frontend"

	// Context-derived fields (from task.Context)
	FamilyHint           string   // From task.Context["euclo.family"]
	UserFiles            []string // From task.Context["euclo.user_files"]
	SessionPins          []string // From task.Context["euclo.session_pins"]
	EditPermitted        bool     // Based on registry write tool availability
	ExplicitVerification bool     // From task.Context["verification"]

	// Resume state (from envelope)
	ResumedFamily           string   // From KeyFamilySelection
	CapabilitySequence      []string // From KeyCapabilitySequence
	NegativeConstraintSeeds []string // Extracted from instruction

	// File and ingestion directives
	ExplicitFiles    []string // File paths mentioned in the message
	IngestPolicy     string   // "files_only", "incremental", "full"
	IncrementalSince string   // Commit hash for incremental ingestion

	// Normalized message (instruction with hints removed)
	CleanMessage string

	// Metadata
	RawMessage string
	Metadata   map[string]any
}
