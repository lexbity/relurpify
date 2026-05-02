package analysis

import (
	"context"
	"fmt"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/agents/goalcon/types"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// ClassificationResponse is defined in classification_prompt.go
// (importing here for clarity in documentation)

// ClarificationChoice represents a user's decision on how to clarify an ambiguity.
type ClarificationChoice struct {
	AmbiguityIndex   int    // Index in the ambiguity list
	ChosenSuggestion string // The clarification suggestion selected
	AlternativeText  string // User-provided custom clarification (if not using suggestion)
	Timestamp        time.Time
	ApprovedBy       string // User who made the decision
	GrantScope       authorization.GrantScope
}

// ClarificationSession tracks a goal clarification interaction.
type ClarificationSession struct {
	ID             string
	Goal           types.GoalCondition
	AmbiguityScore *AmbiguityScore
	HITLRequestIDs []string // Tracking IDs for HITL permission requests
	Choices        []*ClarificationChoice
	StartTime      time.Time
	EndTime        time.Time
	IsComplete     bool
	RefinedGoal    *types.GoalCondition // Result after clarification
	mu             sync.Mutex
}

// GoalClarifier orchestrates goal clarification through HITL.
type GoalClarifier struct {
	hitlBroker        *authorization.HITLBroker
	memoryStore       *memory.WorkingMemoryStore
	analyzer          *AmbiguityAnalyzer
	highRiskThreshold float32 // Threshold to require HITL (default 0.75)
}

// NewGoalClarifier creates a clarifier with HITL and memory store integration.
func NewGoalClarifier(
	hitlBroker *authorization.HITLBroker,
	memoryStore *memory.WorkingMemoryStore,
	analyzer *AmbiguityAnalyzer,
) *GoalClarifier {
	if analyzer == nil {
		analyzer = NewAmbiguityAnalyzer()
	}
	return &GoalClarifier{
		hitlBroker:        hitlBroker,
		memoryStore:       memoryStore,
		analyzer:          analyzer,
		highRiskThreshold: 0.75,
	}
}

// SetHighRiskThreshold sets the ambiguity score threshold requiring HITL.
func (gc *GoalClarifier) SetHighRiskThreshold(threshold float32) {
	if gc != nil && threshold >= 0 && threshold <= 1.0 {
		gc.highRiskThreshold = threshold
	}
}

// ClarifyGoalIfNeeded analyzes a goal and triggers clarification if ambiguous.
// Returns clarified goal if refinement occurred, or original goal if no clarification needed.
func (gc *GoalClarifier) ClarifyGoalIfNeeded(
	ctx context.Context,
	goal types.GoalCondition,
	response *ClassificationResponse,
	planningContext *contextdata.Envelope,
) (types.GoalCondition, *ClarificationSession, error) {
	if gc == nil || gc.analyzer == nil {
		return goal, nil, nil
	}

	// Analyze ambiguities
	score := gc.analyzer.AnalyzeAmbiguities(goal, response)
	if score == nil || !score.ShouldRefine {
		// No clarification needed
		return goal, nil, nil
	}

	// Create clarification session
	session := &ClarificationSession{
		ID:             fmt.Sprintf("clarify-%d", time.Now().UnixNano()),
		Goal:           goal,
		AmbiguityScore: score,
		HITLRequestIDs: make([]string, 0),
		Choices:        make([]*ClarificationChoice, 0),
		StartTime:      time.Now().UTC(),
	}

	// Request HITL for high-ambiguity goals
	if score.OverallScore >= gc.highRiskThreshold {
		requestID, err := gc.requestHITLClarification(ctx, session)
		if err != nil {
			return goal, session, fmt.Errorf("HITL clarification request failed: %w", err)
		}
		session.HITLRequestIDs = append(session.HITLRequestIDs, requestID)
	}

	// Collect suggestions for context storage
	suggestions := gc.analyzer.GetRefinementSuggestions(score)
	session.StoreClues(goal, suggestions, planningContext)

	return goal, session, nil
}

// requestHITLClarification submits clarification request to HITL broker.
func (gc *GoalClarifier) requestHITLClarification(
	ctx context.Context,
	session *ClarificationSession,
) (string, error) {
	if gc.hitlBroker == nil {
		// If no HITL broker, just mark session complete (non-blocking)
		return "", nil
	}

	// Build human-friendly clarification request
	justification := fmt.Sprintf(
		"Goal '%s' has ambiguities requiring clarification (score: %.0f%%). ",
		session.Goal.Description,
		session.AmbiguityScore.OverallScore*100,
	)
	if session.AmbiguityScore.Reason != "" {
		justification += session.AmbiguityScore.Reason
	}

	// Submit async (non-blocking) so we can continue with fallback
	req := authorization.PermissionRequest{
		Permission: contracts.PermissionDescriptor{
			Action:   "goal_clarification",
			Resource: fmt.Sprintf("goal:%s", session.Goal.Description),
		},
		Justification:   justification,
		Scope:           authorization.GrantScopeTask,
		Risk:            gc.riskLevelForScore(session.AmbiguityScore.OverallScore),
		Timeout:         2 * time.Minute,
		TimeoutBehavior: authorization.HITLTimeoutBehaviorSkip,
	}

	requestID, err := gc.hitlBroker.SubmitAsync(req)
	return requestID, err
}

// riskLevelForScore maps ambiguity score to risk level.
func (gc *GoalClarifier) riskLevelForScore(score float32) authorization.RiskLevel {
	switch {
	case score < 0.4:
		return authorization.RiskLevelLow
	case score < 0.7:
		return authorization.RiskLevelMedium
	default:
		return authorization.RiskLevelHigh
	}
}

// StoreClues stores clarification clues in context and memory.
func (session *ClarificationSession) StoreClues(
	goal types.GoalCondition,
	suggestions []string,
	planningContext *contextdata.Envelope,
) {
	if session == nil || planningContext == nil {
		return
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Store in planning context for downstream steps using SetWorkingValue method
	planningContext.SetWorkingValue("clarification_session_id", session.ID, contextdata.MemoryClassTask)
	planningContext.SetWorkingValue("goal_ambiguities", session.AmbiguityScore.Indicators, contextdata.MemoryClassTask)
	planningContext.SetWorkingValue("clarification_suggestions", suggestions, contextdata.MemoryClassTask)
}

// RecordChoice records a user's clarification decision.
func (session *ClarificationSession) RecordChoice(choice *ClarificationChoice) {
	if session == nil || choice == nil {
		return
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	choice.Timestamp = time.Now().UTC()
	session.Choices = append(session.Choices, choice)
}

// ApplyChoices updates the goal based on clarification choices.
// Returns the refined goal.
func (session *ClarificationSession) ApplyChoices() *types.GoalCondition {
	if session == nil || len(session.Choices) == 0 {
		return &session.Goal
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	refined := session.Goal

	// Update description with clarifications
	var clarifications []string
	for _, choice := range session.Choices {
		if choice.AlternativeText != "" {
			clarifications = append(clarifications, choice.AlternativeText)
		}
	}

	if len(clarifications) > 0 {
		refined.Description = session.Goal.Description + " (clarified: " + fmt.Sprintf("%v", clarifications) + ")"
	}

	session.RefinedGoal = &refined
	session.IsComplete = true
	session.EndTime = time.Now().UTC()

	return &refined
}

// PersistToMemory saves clarification session to memory store.
func (session *ClarificationSession) PersistToMemory(
	ctx context.Context,
	ms *memory.WorkingMemoryStore,
	planID string,
) error {
	if session == nil || ms == nil {
		return nil
	}

	if ctx == nil {
		ctx = context.Background()
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Build memory entry
	data := map[string]interface{}{
		"session_id":      session.ID,
		"goal":            session.Goal.Description,
		"ambiguity_score": session.AmbiguityScore.OverallScore,
		"num_ambiguities": len(session.AmbiguityScore.Indicators),
		"num_choices":     len(session.Choices),
		"is_complete":     session.IsComplete,
		"duration_ms":     session.EndTime.Sub(session.StartTime).Milliseconds(),
		"start_time":      session.StartTime,
		"end_time":        session.EndTime,
	}

	if session.RefinedGoal != nil {
		data["refined_goal"] = session.RefinedGoal.Description
	}

	// Store in memory with project scope for plan-level persistence
	// Key format: "clarification:<session_id>:<plan_id>"
	key := fmt.Sprintf("clarification:%s:%s", session.ID, planID)
	ms.Scope("goalcon").Set(key, data, core.MemoryClassWorking)
	return nil
}

// FormatSessionSummary generates a human-readable summary of the clarification session.
func (session *ClarificationSession) FormatSessionSummary() string {
	if session == nil {
		return "No clarification session"
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	summary := fmt.Sprintf("Goal Clarification Session %s\n", session.ID)
	summary += fmt.Sprintf("=====================================\n")
	summary += fmt.Sprintf("Original Goal: %s\n", session.Goal.Description)
	summary += fmt.Sprintf("Ambiguity Score: %.0f%%\n", session.AmbiguityScore.OverallScore*100)
	summary += fmt.Sprintf("Total Ambiguities Found: %d\n", len(session.AmbiguityScore.Indicators))

	if len(session.Choices) > 0 {
		summary += fmt.Sprintf("\nClarifications Made:\n")
		for i, choice := range session.Choices {
			summary += fmt.Sprintf("  %d. %s\n", i+1, choice.ChosenSuggestion)
			if choice.AlternativeText != "" {
				summary += fmt.Sprintf("     (custom: %s)\n", choice.AlternativeText)
			}
		}
	}

	if session.RefinedGoal != nil {
		summary += fmt.Sprintf("\nRefined Goal: %s\n", session.RefinedGoal.Description)
	}

	duration := session.EndTime.Sub(session.StartTime)
	if duration > 0 {
		summary += fmt.Sprintf("Duration: %v\n", duration)
	}

	return summary
}
