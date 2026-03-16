package audit

import (
	"github.com/lexcodex/relurpify/agents/goalcon/types"
)

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// AuditEntry represents a single capability invocation in the audit trail.
type AuditEntry struct {
	// Identity
	ID            string    `json:"id" yaml:"id"`
	Timestamp     time.Time `json:"timestamp" yaml:"timestamp"`
	StepID        string    `json:"step_id,omitempty" yaml:"step_id,omitempty"`
	CapabilityID  string    `json:"capability_id" yaml:"capability_id"`
	CapabilityName string   `json:"capability_name" yaml:"capability_name"`

	// Classification
	TrustClass    core.TrustClass     `json:"trust_class" yaml:"trust_class"`
	EffectClasses []core.EffectClass  `json:"effect_classes,omitempty" yaml:"effect_classes,omitempty"`
	RiskClasses   []core.RiskClass    `json:"risk_classes,omitempty" yaml:"risk_classes,omitempty"`

	// Execution
	InputSummary  string `json:"input_summary,omitempty" yaml:"input_summary,omitempty"`
	OutputSummary string `json:"output_summary,omitempty" yaml:"output_summary,omitempty"`
	Success       bool   `json:"success" yaml:"success"`
	ErrorMessage  string `json:"error_message,omitempty" yaml:"error_message,omitempty"`
	Duration      int64  `json:"duration_ms" yaml:"duration_ms"`

	// Policy & Approval
	InsertionAction InsertionAction     `json:"insertion_action" yaml:"insertion_action"`
	InsertionReason string              `json:"insertion_reason,omitempty" yaml:"insertion_reason,omitempty"`
	PolicySnapshot  *core.PolicySnapshot `json:"policy_snapshot,omitempty" yaml:"policy_snapshot,omitempty"`
	ApprovalBinding *core.ApprovalBinding `json:"approval_binding,omitempty" yaml:"approval_binding,omitempty"`
}

// InsertionAction mirrors core.InsertionAction for local use
type InsertionAction string

const (
	InsertionActionDirect       InsertionAction = "direct"
	InsertionActionSummarized   InsertionAction = "summarized"
	InsertionActionMetadataOnly InsertionAction = "metadata-only"
	InsertionActionHITLRequired InsertionAction = "hitl-required"
	InsertionActionDenied       InsertionAction = "denied"
)

// types.CapabilityAuditTrail tracks all capability invocations during plan execution.
type types.CapabilityAuditTrail struct {
	planID  string
	agentID string
	entries []*AuditEntry
	mu      sync.RWMutex
}

// NewCapabilityAuditTrail creates a new audit trail for a plan execution.
func NewCapabilityAuditTrail(planID string) *types.CapabilityAuditTrail {
	return &types.CapabilityAuditTrail{
		planID:  planID,
		entries: make([]*AuditEntry, 0, 10),
	}
}

// SetAgentID sets the agent ID that is executing the plan.
func (t *types.CapabilityAuditTrail) SetAgentID(agentID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.agentID = agentID
}

// RecordInvocation records a capability invocation from a result envelope.
func (t *types.CapabilityAuditTrail) RecordInvocation(stepID string, envelope *core.CapabilityResultEnvelope, decision core.InsertionDecision) {
	if t == nil || envelope == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Generate deterministic ID from step + capability + timestamp
	idInput := fmt.Sprintf("%s:%s:%d", stepID, envelope.Descriptor.ID, envelope.RecordedAt.UnixNano())
	idHash := sha256.Sum256([]byte(idInput))
	entryID := hex.EncodeToString(idHash[:])[:16] // First 16 chars of hex hash

	entry := &AuditEntry{
		ID:              entryID,
		Timestamp:       envelope.RecordedAt,
		StepID:          stepID,
		CapabilityID:    envelope.Descriptor.ID,
		CapabilityName:  envelope.Descriptor.Name,
		TrustClass:      envelope.Descriptor.TrustClass,
		EffectClasses:   envelope.Descriptor.EffectClasses,
		RiskClasses:     envelope.Descriptor.RiskClasses,
		Success:         envelope.Result != nil && envelope.Result.Success,
		ErrorMessage:    "",
		Duration:        0,
		InsertionAction: InsertionAction(decision.Action),
		InsertionReason: decision.Reason,
		PolicySnapshot:  envelope.Policy,
		ApprovalBinding: envelope.Approval,
	}

	// Summarize input/output (first 200 chars to avoid bloat)
	if envelope.Result != nil {
		if envelope.Result.Error != "" {
			entry.ErrorMessage = envelope.Result.Error
		}
		if len(envelope.Result.Data) > 0 {
			dataJSON, _ := json.Marshal(envelope.Result.Data)
			entry.OutputSummary = truncate(string(dataJSON), 200)
		}
	}

	// Extract duration if available in result metadata
	if envelope.Result != nil && envelope.Result.Metadata != nil {
		if durMs, ok := envelope.Result.Metadata["duration_ms"].(float64); ok {
			entry.Duration = int64(durMs)
		}
	}

	t.entries = append(t.entries, entry)
}

// GetEntries returns all audit entries in time order.
func (t *types.CapabilityAuditTrail) GetEntries() []*AuditEntry {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]*AuditEntry, len(t.entries))
	copy(result, t.entries)
	return result
}

// GetEntriesByCapability returns all invocations of a specific capability.
func (t *types.CapabilityAuditTrail) GetEntriesByCapability(capabilityID string) []*AuditEntry {
	if t == nil || capabilityID == "" {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()

	var result []*AuditEntry
	for _, entry := range t.entries {
		if entry.CapabilityID == capabilityID {
			result = append(result, entry)
		}
	}
	return result
}

// GetEntriesByTrustClass returns all invocations with a specific trust class.
func (t *types.CapabilityAuditTrail) GetEntriesByTrustClass(trustClass core.TrustClass) []*AuditEntry {
	if t == nil || trustClass == "" {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()

	var result []*AuditEntry
	for _, entry := range t.entries {
		if entry.TrustClass == trustClass {
			result = append(result, entry)
		}
	}
	return result
}

// GetEntriesByInsertion returns all entries with a specific insertion action.
func (t *types.CapabilityAuditTrail) GetEntriesByInsertion(action InsertionAction) []*AuditEntry {
	if t == nil || action == "" {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()

	var result []*AuditEntry
	for _, entry := range t.entries {
		if entry.InsertionAction == action {
			result = append(result, entry)
		}
	}
	return result
}

// Summary returns aggregated statistics about the audit trail.
type AuditSummary struct {
	TotalInvocations   int                         `json:"total_invocations" yaml:"total_invocations"`
	SuccessfulCount    int                         `json:"successful_count" yaml:"successful_count"`
	FailedCount        int                         `json:"failed_count" yaml:"failed_count"`
	TotalDurationMS    int64                       `json:"total_duration_ms" yaml:"total_duration_ms"`
	TrustDistribution  map[string]int              `json:"trust_distribution" yaml:"trust_distribution"`
	InsertionDistribution map[string]int           `json:"insertion_distribution" yaml:"insertion_distribution"`
	UniqueCapabilities int                         `json:"unique_capabilities" yaml:"unique_capabilities"`
}

// Summary generates aggregated statistics about all recorded invocations.
func (t *types.CapabilityAuditTrail) Summary() AuditSummary {
	if t == nil {
		return AuditSummary{}
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	summary := AuditSummary{
		TotalInvocations:      len(t.entries),
		TrustDistribution:     make(map[string]int),
		InsertionDistribution: make(map[string]int),
	}

	uniqueCaps := make(map[string]struct{})

	for _, entry := range t.entries {
		if entry.Success {
			summary.SuccessfulCount++
		} else {
			summary.FailedCount++
		}
		summary.TotalDurationMS += entry.Duration

		// Trust distribution
		trustStr := string(entry.TrustClass)
		if trustStr == "" {
			trustStr = "unknown"
		}
		summary.TrustDistribution[trustStr]++

		// Insertion distribution
		insertionStr := string(entry.InsertionAction)
		if insertionStr == "" {
			insertionStr = "unknown"
		}
		summary.InsertionDistribution[insertionStr]++

		// Track unique capabilities
		if entry.CapabilityID != "" {
			uniqueCaps[entry.CapabilityID] = struct{}{}
		}
	}

	summary.UniqueCapabilities = len(uniqueCaps)
	return summary
}

// ToJSON serializes the audit trail to JSON.
func (t *types.CapabilityAuditTrail) ToJSON() (string, error) {
	if t == nil {
		return "null", nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	data := map[string]any{
		"plan_id":  t.planID,
		"agent_id": t.agentID,
		"entries":  t.entries,
		"summary":  t.summaryLocked(),
	}

	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal audit trail: %w", err)
	}
	return string(jsonBytes), nil
}

// FromJSON deserializes an audit trail from JSON.
func FromJSON(jsonStr string) (*types.CapabilityAuditTrail, error) {
	var data map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, fmt.Errorf("unmarshal audit trail: %w", err)
	}

	planID, _ := data["plan_id"].(string)
	trail := NewCapabilityAuditTrail(planID)

	if agentID, ok := data["agent_id"].(string); ok {
		trail.SetAgentID(agentID)
	}

	if entriesData, ok := data["entries"].([]any); ok {
		for _, entryData := range entriesData {
			entryJSON, _ := json.Marshal(entryData)
			var entry AuditEntry
			if err := json.Unmarshal(entryJSON, &entry); err != nil {
				continue
			}
			trail.mu.Lock()
			trail.entries = append(trail.entries, &entry)
			trail.mu.Unlock()
		}
	}

	return trail, nil
}

// summaryLocked is an internal version of Summary for use within mutex-protected sections.
func (t *types.CapabilityAuditTrail) summaryLocked() AuditSummary {
	summary := AuditSummary{
		TotalInvocations:      len(t.entries),
		TrustDistribution:     make(map[string]int),
		InsertionDistribution: make(map[string]int),
	}

	uniqueCaps := make(map[string]struct{})

	for _, entry := range t.entries {
		if entry.Success {
			summary.SuccessfulCount++
		} else {
			summary.FailedCount++
		}
		summary.TotalDurationMS += entry.Duration

		trustStr := string(entry.TrustClass)
		if trustStr == "" {
			trustStr = "unknown"
		}
		summary.TrustDistribution[trustStr]++

		insertionStr := string(entry.InsertionAction)
		if insertionStr == "" {
			insertionStr = "unknown"
		}
		summary.InsertionDistribution[insertionStr]++

		if entry.CapabilityID != "" {
			uniqueCaps[entry.CapabilityID] = struct{}{}
		}
	}

	summary.UniqueCapabilities = len(uniqueCaps)
	return summary
}

// truncate returns the first n characters of a string
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
