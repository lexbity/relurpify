package blackboard

import "time"

// Fact captures a confirmed piece of information added to the blackboard.
type Fact struct {
	Key       string
	Value     string
	Source    string    // which KS produced this
	CreatedAt time.Time
}

// Hypothesis represents a candidate partial solution under consideration.
type Hypothesis struct {
	ID          string
	Description string
	Confidence  float64 // 0–1
	Source      string
	CreatedAt   time.Time
}

// ActionRequest is a queued tool or agent invocation.
type ActionRequest struct {
	ID           string
	ToolOrAgent  string
	Args         map[string]any
	Description  string
	RequestedBy  string // KS name
	CreatedAt    time.Time
}

// ActionResult captures the outcome of an executed ActionRequest.
type ActionResult struct {
	RequestID string
	Success   bool
	Output    string
	Error     string
	CreatedAt time.Time
}

// Issue describes a problem or gap identified by the Analyzer KS.
type Issue struct {
	ID          string
	Description string
	Severity    string // "low" | "medium" | "high"
	Source      string
	CreatedAt   time.Time
}

// Artifact is a produced output such as a patch, summary, or document.
type Artifact struct {
	ID         string
	Kind       string   // "patch" | "summary" | "report" | etc.
	Content    string
	Verified   bool
	Source     string
	CreatedAt  time.Time
}

// Blackboard is the shared in-memory workspace for the blackboard architecture.
// All fields are exported so knowledge sources can read and mutate them directly.
type Blackboard struct {
	// Goals lists what the agent is trying to achieve.
	Goals []string
	// Facts holds confirmed information gathered from the workspace.
	Facts []Fact
	// Hypotheses holds candidate partial solutions.
	Hypotheses []Hypothesis
	// PendingActions holds queued tool/agent invocations.
	PendingActions []ActionRequest
	// CompletedActions holds the results of executed actions.
	CompletedActions []ActionResult
	// Issues holds problems identified during analysis.
	Issues []Issue
	// Artifacts holds produced outputs.
	Artifacts []Artifact
}

// NewBlackboard returns an empty blackboard with the given goals.
func NewBlackboard(goals ...string) *Blackboard {
	return &Blackboard{
		Goals: goals,
	}
}

// IsGoalSatisfied returns true when at least one unverified artifact exists,
// indicating that the KS loop has produced output to be verified, or when at
// least one verified artifact exists.
func (bb *Blackboard) IsGoalSatisfied() bool {
	for _, a := range bb.Artifacts {
		if a.Verified {
			return true
		}
	}
	return false
}

// HasUnverifiedArtifacts returns true when artifacts are present but none
// are marked verified.
func (bb *Blackboard) HasUnverifiedArtifacts() bool {
	if len(bb.Artifacts) == 0 {
		return false
	}
	for _, a := range bb.Artifacts {
		if a.Verified {
			return false
		}
	}
	return true
}

// AddFact appends a fact to the blackboard.
func (bb *Blackboard) AddFact(key, value, source string) {
	bb.Facts = append(bb.Facts, Fact{
		Key:       key,
		Value:     value,
		Source:    source,
		CreatedAt: time.Now().UTC(),
	})
}

// AddIssue appends an issue to the blackboard.
func (bb *Blackboard) AddIssue(id, description, severity, source string) {
	bb.Issues = append(bb.Issues, Issue{
		ID:          id,
		Description: description,
		Severity:    severity,
		Source:      source,
		CreatedAt:   time.Now().UTC(),
	})
}

// AddArtifact appends an artifact to the blackboard.
func (bb *Blackboard) AddArtifact(id, kind, content, source string) {
	bb.Artifacts = append(bb.Artifacts, Artifact{
		ID:        id,
		Kind:      kind,
		Content:   content,
		Source:    source,
		CreatedAt: time.Now().UTC(),
	})
}
