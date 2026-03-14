package blackboard

import (
	"fmt"
	"strings"
	"time"
)

const BlackboardStateSchemaVersion = 1

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

// ControllerState captures controller-owned runtime metadata exposed through
// core.Context alongside the shared blackboard lanes.
type ControllerState struct {
	Cycle           int       `json:"cycle"`
	MaxCycles       int       `json:"max_cycles"`
	Termination     string    `json:"termination"`
	LastSource      string    `json:"last_source,omitempty"`
	LastUpdatedAt   time.Time `json:"last_updated_at"`
	GoalSatisfied   bool      `json:"goal_satisfied"`
	PrototypeCompat bool      `json:"prototype_compat"`
}

// Metrics summarizes the current blackboard contents for fast inspection.
type Metrics struct {
	GoalCount       int `json:"goal_count"`
	FactCount       int `json:"fact_count"`
	HypothesisCount int `json:"hypothesis_count"`
	IssueCount      int `json:"issue_count"`
	PendingCount    int `json:"pending_count"`
	CompletedCount  int `json:"completed_count"`
	ArtifactCount   int `json:"artifact_count"`
	VerifiedCount   int `json:"verified_count"`
}

// Blackboard is the shared in-memory workspace for the blackboard architecture.
// All fields are exported so knowledge sources can read and mutate them directly.
type Blackboard struct {
	// SchemaVersion tracks the serialized blackboard state shape.
	SchemaVersion int
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
		SchemaVersion: BlackboardStateSchemaVersion,
		Goals:         append([]string(nil), goals...),
	}
}

// Clone returns a detached copy suitable for publishing into core.Context.
func (bb *Blackboard) Clone() *Blackboard {
	if bb == nil {
		return nil
	}
	clone := &Blackboard{
		SchemaVersion:    bb.SchemaVersion,
		Goals:            append([]string(nil), bb.Goals...),
		Facts:            append([]Fact(nil), bb.Facts...),
		Hypotheses:       append([]Hypothesis(nil), bb.Hypotheses...),
		CompletedActions: append([]ActionResult(nil), bb.CompletedActions...),
		Issues:           append([]Issue(nil), bb.Issues...),
		Artifacts:        append([]Artifact(nil), bb.Artifacts...),
	}
	if len(bb.PendingActions) > 0 {
		clone.PendingActions = make([]ActionRequest, len(bb.PendingActions))
		for i, req := range bb.PendingActions {
			clone.PendingActions[i] = ActionRequest{
				ID:          req.ID,
				ToolOrAgent: req.ToolOrAgent,
				Args:        cloneMap(req.Args),
				Description: req.Description,
				RequestedBy: req.RequestedBy,
				CreatedAt:   req.CreatedAt,
			}
		}
	}
	return clone
}

// Normalize fills in default state metadata after hydration.
func (bb *Blackboard) Normalize() {
	if bb == nil {
		return
	}
	if bb.SchemaVersion == 0 {
		bb.SchemaVersion = BlackboardStateSchemaVersion
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

// SetGoals replaces the goal set with a detached copy.
func (bb *Blackboard) SetGoals(goals ...string) {
	if bb == nil {
		return
	}
	bb.Goals = append([]string(nil), goals...)
}

// AddFact appends a fact to the blackboard unless an equivalent fact already exists.
func (bb *Blackboard) AddFact(key, value, source string) bool {
	if bb == nil {
		return false
	}
	key = strings.TrimSpace(key)
	source = strings.TrimSpace(source)
	if key == "" {
		return false
	}
	for _, existing := range bb.Facts {
		if existing.Key == key && existing.Value == value && existing.Source == source {
			return false
		}
	}
	bb.Facts = append(bb.Facts, Fact{
		Key:       key,
		Value:     value,
		Source:    source,
		CreatedAt: time.Now().UTC(),
	})
	return true
}

// AddHypothesis appends a candidate hypothesis when it passes validation.
func (bb *Blackboard) AddHypothesis(id, description string, confidence float64, source string) error {
	if bb == nil {
		return fmt.Errorf("blackboard required")
	}
	id = strings.TrimSpace(id)
	description = strings.TrimSpace(description)
	source = strings.TrimSpace(source)
	if id == "" {
		return fmt.Errorf("hypothesis id required")
	}
	if description == "" {
		return fmt.Errorf("hypothesis description required")
	}
	if confidence < 0 || confidence > 1 {
		return fmt.Errorf("hypothesis confidence must be between 0 and 1")
	}
	if bb.HasHypothesis(id) {
		return fmt.Errorf("hypothesis %q already exists", id)
	}
	bb.Hypotheses = append(bb.Hypotheses, Hypothesis{
		ID:          id,
		Description: description,
		Confidence:  confidence,
		Source:      source,
		CreatedAt:   time.Now().UTC(),
	})
	return nil
}

// AddIssue appends an issue to the blackboard.
func (bb *Blackboard) AddIssue(id, description, severity, source string) error {
	if bb == nil {
		return fmt.Errorf("blackboard required")
	}
	id = strings.TrimSpace(id)
	description = strings.TrimSpace(description)
	source = strings.TrimSpace(source)
	severity = normalizeSeverity(severity)
	if id == "" {
		return fmt.Errorf("issue id required")
	}
	if description == "" {
		return fmt.Errorf("issue description required")
	}
	if !isValidSeverity(severity) {
		return fmt.Errorf("issue severity %q invalid", severity)
	}
	if bb.HasIssue(id) {
		return fmt.Errorf("issue %q already exists", id)
	}
	bb.Issues = append(bb.Issues, Issue{
		ID:          id,
		Description: description,
		Severity:    severity,
		Source:      source,
		CreatedAt:   time.Now().UTC(),
	})
	return nil
}

// EnqueueAction appends a pending action request when it passes validation.
func (bb *Blackboard) EnqueueAction(req ActionRequest) error {
	if bb == nil {
		return fmt.Errorf("blackboard required")
	}
	req.ID = strings.TrimSpace(req.ID)
	req.ToolOrAgent = strings.TrimSpace(req.ToolOrAgent)
	req.Description = strings.TrimSpace(req.Description)
	req.RequestedBy = strings.TrimSpace(req.RequestedBy)
	if req.ID == "" {
		return fmt.Errorf("action id required")
	}
	if req.ToolOrAgent == "" {
		return fmt.Errorf("action tool or agent required")
	}
	if bb.HasPendingAction(req.ID) || bb.HasCompletedAction(req.ID) {
		return fmt.Errorf("action %q already exists", req.ID)
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}
	req.Args = cloneMap(req.Args)
	bb.PendingActions = append(bb.PendingActions, req)
	return nil
}

// CompleteAction records an action result and removes the matching pending request.
func (bb *Blackboard) CompleteAction(result ActionResult) error {
	if bb == nil {
		return fmt.Errorf("blackboard required")
	}
	result.RequestID = strings.TrimSpace(result.RequestID)
	if result.RequestID == "" {
		return fmt.Errorf("action result request id required")
	}
	if bb.HasCompletedAction(result.RequestID) {
		return fmt.Errorf("action result %q already recorded", result.RequestID)
	}
	if result.CreatedAt.IsZero() {
		result.CreatedAt = time.Now().UTC()
	}
	bb.CompletedActions = append(bb.CompletedActions, result)
	if idx := bb.pendingActionIndex(result.RequestID); idx >= 0 {
		bb.PendingActions = append(bb.PendingActions[:idx], bb.PendingActions[idx+1:]...)
	}
	return nil
}

// AddArtifact appends an artifact to the blackboard.
func (bb *Blackboard) AddArtifact(id, kind, content, source string) error {
	if bb == nil {
		return fmt.Errorf("blackboard required")
	}
	id = strings.TrimSpace(id)
	kind = strings.TrimSpace(kind)
	source = strings.TrimSpace(source)
	if id == "" {
		return fmt.Errorf("artifact id required")
	}
	if kind == "" {
		return fmt.Errorf("artifact kind required")
	}
	if bb.HasArtifact(id) {
		return fmt.Errorf("artifact %q already exists", id)
	}
	bb.Artifacts = append(bb.Artifacts, Artifact{
		ID:        id,
		Kind:      kind,
		Content:   content,
		Source:    source,
		CreatedAt: time.Now().UTC(),
	})
	return nil
}

// VerifyArtifact marks one artifact verified.
func (bb *Blackboard) VerifyArtifact(id string) bool {
	if bb == nil {
		return false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for i := range bb.Artifacts {
		if bb.Artifacts[i].ID != id {
			continue
		}
		if bb.Artifacts[i].Verified {
			return false
		}
		bb.Artifacts[i].Verified = true
		return true
	}
	return false
}

// VerifyAllArtifacts marks all current artifacts verified and returns the number changed.
func (bb *Blackboard) VerifyAllArtifacts() int {
	if bb == nil {
		return 0
	}
	verified := 0
	for i := range bb.Artifacts {
		if bb.Artifacts[i].Verified {
			continue
		}
		bb.Artifacts[i].Verified = true
		verified++
	}
	return verified
}

// Validate checks typed-state invariants needed for context publication and recovery.
func (bb *Blackboard) Validate() error {
	if bb == nil {
		return fmt.Errorf("blackboard required")
	}
	if bb.SchemaVersion == 0 {
		return fmt.Errorf("schema version required")
	}
	seenIssues := make(map[string]struct{}, len(bb.Issues))
	for _, issue := range bb.Issues {
		if issue.ID == "" {
			return fmt.Errorf("issue id required")
		}
		if !isValidSeverity(normalizeSeverity(issue.Severity)) {
			return fmt.Errorf("issue severity %q invalid", issue.Severity)
		}
		if _, exists := seenIssues[issue.ID]; exists {
			return fmt.Errorf("duplicate issue id %q", issue.ID)
		}
		seenIssues[issue.ID] = struct{}{}
	}
	seenPending := make(map[string]struct{}, len(bb.PendingActions))
	for _, action := range bb.PendingActions {
		if action.ID == "" {
			return fmt.Errorf("pending action id required")
		}
		if action.ToolOrAgent == "" {
			return fmt.Errorf("pending action %q tool or agent required", action.ID)
		}
		if _, exists := seenPending[action.ID]; exists {
			return fmt.Errorf("duplicate pending action id %q", action.ID)
		}
		seenPending[action.ID] = struct{}{}
	}
	seenCompleted := make(map[string]struct{}, len(bb.CompletedActions))
	for _, result := range bb.CompletedActions {
		if result.RequestID == "" {
			return fmt.Errorf("completed action request id required")
		}
		if _, exists := seenCompleted[result.RequestID]; exists {
			return fmt.Errorf("duplicate completed action id %q", result.RequestID)
		}
		seenCompleted[result.RequestID] = struct{}{}
	}
	seenArtifacts := make(map[string]struct{}, len(bb.Artifacts))
	for _, artifact := range bb.Artifacts {
		if artifact.ID == "" {
			return fmt.Errorf("artifact id required")
		}
		if artifact.Kind == "" {
			return fmt.Errorf("artifact %q kind required", artifact.ID)
		}
		if _, exists := seenArtifacts[artifact.ID]; exists {
			return fmt.Errorf("duplicate artifact id %q", artifact.ID)
		}
		seenArtifacts[artifact.ID] = struct{}{}
	}
	return nil
}

func (bb *Blackboard) HasIssue(id string) bool {
	if bb == nil {
		return false
	}
	id = strings.TrimSpace(id)
	for _, issue := range bb.Issues {
		if issue.ID == id {
			return true
		}
	}
	return false
}

func (bb *Blackboard) HasHypothesis(id string) bool {
	if bb == nil {
		return false
	}
	id = strings.TrimSpace(id)
	for _, hypothesis := range bb.Hypotheses {
		if hypothesis.ID == id {
			return true
		}
	}
	return false
}

func (bb *Blackboard) HasPendingAction(id string) bool {
	return bb.pendingActionIndex(id) >= 0
}

func (bb *Blackboard) HasCompletedAction(id string) bool {
	if bb == nil {
		return false
	}
	id = strings.TrimSpace(id)
	for _, result := range bb.CompletedActions {
		if result.RequestID == id {
			return true
		}
	}
	return false
}

func (bb *Blackboard) HasArtifact(id string) bool {
	if bb == nil {
		return false
	}
	id = strings.TrimSpace(id)
	for _, artifact := range bb.Artifacts {
		if artifact.ID == id {
			return true
		}
	}
	return false
}

func (bb *Blackboard) pendingActionIndex(id string) int {
	if bb == nil {
		return -1
	}
	id = strings.TrimSpace(id)
	for i, action := range bb.PendingActions {
		if action.ID == id {
			return i
		}
	}
	return -1
}

func normalizeSeverity(severity string) string {
	return strings.ToLower(strings.TrimSpace(severity))
}

func isValidSeverity(severity string) bool {
	switch severity {
	case "low", "medium", "high":
		return true
	default:
		return false
	}
}

// Metrics returns summarized counts for the current blackboard state.
func (bb *Blackboard) Metrics() Metrics {
	if bb == nil {
		return Metrics{}
	}
	verified := 0
	for _, artifact := range bb.Artifacts {
		if artifact.Verified {
			verified++
		}
	}
	return Metrics{
		GoalCount:       len(bb.Goals),
		FactCount:       len(bb.Facts),
		HypothesisCount: len(bb.Hypotheses),
		IssueCount:      len(bb.Issues),
		PendingCount:    len(bb.PendingActions),
		CompletedCount:  len(bb.CompletedActions),
		ArtifactCount:   len(bb.Artifacts),
		VerifiedCount:   verified,
	}
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
