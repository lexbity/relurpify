package audit

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// ProvenanceSummary provides a complete audit and policy compliance view of plan execution.
type ProvenanceSummary struct {
	// Coverage
	PlanID                     string `json:"plan_id,omitempty" yaml:"plan_id,omitempty"`
	AgentID                    string `json:"agent_id,omitempty" yaml:"agent_id,omitempty"`
	TotalCapabilityInvocations int    `json:"total_capability_invocations" yaml:"total_capability_invocations"`
	UniqueCapabilities         int    `json:"unique_capabilities" yaml:"unique_capabilities"`

	// Trust distribution
	TrustDistribution map[string]int `json:"trust_distribution" yaml:"trust_distribution"`

	// Effect & Risk distribution
	EffectDistribution map[string]int `json:"effect_distribution" yaml:"effect_distribution"`
	RiskDistribution   map[string]int `json:"risk_distribution" yaml:"risk_distribution"`

	// Policy enforcement
	InsertionDistribution map[string]int `json:"insertion_distribution" yaml:"insertion_distribution"`
	ApprovedCapabilities  []string       `json:"approved_capabilities,omitempty" yaml:"approved_capabilities,omitempty"`
	DeniedCapabilities    []string       `json:"denied_capabilities,omitempty" yaml:"denied_capabilities,omitempty"`

	// Risk assessment
	HighRiskExecutions []RiskHighlight   `json:"high_risk_executions,omitempty" yaml:"high_risk_executions,omitempty"`
	PolicyViolations   []PolicyViolation `json:"policy_violations,omitempty" yaml:"policy_violations,omitempty"`

	// Execution quality
	SuccessRate     float64 `json:"success_rate" yaml:"success_rate"`
	TotalDurationMS int64   `json:"total_duration_ms" yaml:"total_duration_ms"`

	// Narrative
	HumanSummary string `json:"human_summary" yaml:"human_summary"`

	// Metadata
	GeneratedAt string `json:"generated_at" yaml:"generated_at"`
}

// RiskHighlight identifies executions with security-relevant effect classes.
type RiskHighlight struct {
	StepID          string             `json:"step_id" yaml:"step_id"`
	CapabilityID    string             `json:"capability_id" yaml:"capability_id"`
	CapabilityName  string             `json:"capability_name" yaml:"capability_name"`
	EffectClasses   []core.EffectClass `json:"effect_classes" yaml:"effect_classes"`
	TrustClass      core.TrustClass    `json:"trust_class" yaml:"trust_class"`
	InsertionAction string             `json:"insertion_action" yaml:"insertion_action"`
}

// PolicyViolation identifies execution decisions that require attention.
type PolicyViolation struct {
	StepID         string          `json:"step_id" yaml:"step_id"`
	CapabilityID   string          `json:"capability_id" yaml:"capability_id"`
	CapabilityName string          `json:"capability_name" yaml:"capability_name"`
	ViolationType  string          `json:"violation_type" yaml:"violation_type"` // "denied" or "hitl-required"
	Reason         string          `json:"reason,omitempty" yaml:"reason,omitempty"`
	TrustClass     core.TrustClass `json:"trust_class" yaml:"trust_class"`
}

// ProvenanceCollector builds a provenance summary from plan execution data.
type ProvenanceCollector struct {
	plan       *core.Plan
	trace      interface{}
	auditTrail *CapabilityAuditTrail
}

// NewProvenanceCollector creates a collector for the given execution artifacts.
func NewProvenanceCollector(plan *core.Plan, trace interface{}, auditTrail *CapabilityAuditTrail) *ProvenanceCollector {
	return &ProvenanceCollector{
		plan:       plan,
		trace:      trace,
		auditTrail: auditTrail,
	}
}

// BuildProvenance synthesizes an audit trail, execution trace, and plan into a complete provenance summary.
func (c *ProvenanceCollector) BuildProvenance() ProvenanceSummary {
	if c == nil || c.auditTrail == nil {
		return ProvenanceSummary{}
	}

	summary := ProvenanceSummary{
		TrustDistribution:     make(map[string]int),
		EffectDistribution:    make(map[string]int),
		RiskDistribution:      make(map[string]int),
		InsertionDistribution: make(map[string]int),
		ApprovedCapabilities:  make([]string, 0),
		DeniedCapabilities:    make([]string, 0),
		HighRiskExecutions:    make([]RiskHighlight, 0),
		PolicyViolations:      make([]PolicyViolation, 0),
	}

	// Set IDs
	if c.plan != nil {
		summary.PlanID = c.plan.Goal // Use goal as plan identifier
	}
	summary.AgentID = c.auditTrail.agentID

	// Analyze audit trail
	entries := c.auditTrail.GetEntries()
	summary.TotalCapabilityInvocations = len(entries)

	uniqueCaps := make(map[string]struct{})
	successCount := 0
	var totalDuration int64

	for _, entry := range entries {
		// Track unique capabilities
		if entry.CapabilityID != "" {
			uniqueCaps[entry.CapabilityID] = struct{}{}
		}

		// Success rate
		if entry.Success {
			successCount++
		}
		totalDuration += entry.Duration

		// Trust distribution
		trustStr := string(entry.TrustClass)
		if trustStr == "" {
			trustStr = "unknown"
		}
		summary.TrustDistribution[trustStr]++

		// Effect and risk distribution
		for _, effect := range entry.EffectClasses {
			effectStr := string(effect)
			if effectStr != "" {
				summary.EffectDistribution[effectStr]++
			}
		}
		for _, risk := range entry.RiskClasses {
			riskStr := string(risk)
			if riskStr != "" {
				summary.RiskDistribution[riskStr]++
			}
		}

		// Insertion distribution and approvals
		insertionStr := string(entry.InsertionAction)
		if insertionStr == "" {
			insertionStr = "unknown"
		}
		summary.InsertionDistribution[insertionStr]++

		// Track approved/denied
		if entry.InsertionAction == InsertionActionDirect && entry.ApprovalBinding != nil {
			summary.ApprovedCapabilities = append(summary.ApprovedCapabilities, entry.CapabilityName)
		}
		if entry.InsertionAction == InsertionActionDenied {
			summary.DeniedCapabilities = append(summary.DeniedCapabilities, entry.CapabilityName)
		}

		// High-risk executions (destructive, execute, network effects)
		if c.isHighRisk(entry) {
			summary.HighRiskExecutions = append(summary.HighRiskExecutions, RiskHighlight{
				StepID:          entry.StepID,
				CapabilityID:    entry.CapabilityID,
				CapabilityName:  entry.CapabilityName,
				EffectClasses:   entry.EffectClasses,
				TrustClass:      entry.TrustClass,
				InsertionAction: string(entry.InsertionAction),
			})
		}

		// Policy violations
		if entry.InsertionAction == InsertionActionDenied {
			summary.PolicyViolations = append(summary.PolicyViolations, PolicyViolation{
				StepID:         entry.StepID,
				CapabilityID:   entry.CapabilityID,
				CapabilityName: entry.CapabilityName,
				ViolationType:  "denied",
				Reason:         entry.InsertionReason,
				TrustClass:     entry.TrustClass,
			})
		}
		if entry.InsertionAction == InsertionActionHITLRequired {
			summary.PolicyViolations = append(summary.PolicyViolations, PolicyViolation{
				StepID:         entry.StepID,
				CapabilityID:   entry.CapabilityID,
				CapabilityName: entry.CapabilityName,
				ViolationType:  "hitl-required",
				Reason:         entry.InsertionReason,
				TrustClass:     entry.TrustClass,
			})
		}
	}

	summary.UniqueCapabilities = len(uniqueCaps)
	summary.TotalDurationMS = totalDuration
	if summary.TotalCapabilityInvocations > 0 {
		summary.SuccessRate = float64(successCount) / float64(summary.TotalCapabilityInvocations)
	}

	// Deduplicate approved/denied lists
	summary.ApprovedCapabilities = deduplicate(summary.ApprovedCapabilities)
	summary.DeniedCapabilities = deduplicate(summary.DeniedCapabilities)

	// Generate human-readable summary
	summary.HumanSummary = c.buildHumanSummary(summary)
	summary.GeneratedAt = formatTime()

	return summary
}

// isHighRisk determines if an audit entry represents a high-risk execution.
func (c *ProvenanceCollector) isHighRisk(entry *AuditEntry) bool {
	// Check for destructive, execute, or network effects
	destructiveEffects := map[core.EffectClass]bool{
		core.EffectClassFilesystemMutation: true,
		core.EffectClassProcessSpawn:       true,
		core.EffectClassNetworkEgress:      true,
		core.EffectClassExternalState:      true,
	}

	for _, effect := range entry.EffectClasses {
		if destructiveEffects[effect] {
			return true
		}
	}

	// Also flag untrusted capabilities
	untrustedClasses := map[core.TrustClass]bool{
		core.TrustClassProviderLocalUntrusted: true,
		core.TrustClassRemoteDeclared:         true,
	}

	if untrustedClasses[entry.TrustClass] {
		return true
	}

	return false
}

// buildHumanSummary generates a narrative description of the provenance.
func (c *ProvenanceCollector) buildHumanSummary(summary ProvenanceSummary) string {
	var lines []string

	lines = append(lines, fmt.Sprintf("Execution Plan: %s", summary.PlanID))
	lines = append(lines, fmt.Sprintf("Capabilities Invoked: %d (unique: %d)", summary.TotalCapabilityInvocations, summary.UniqueCapabilities))
	lines = append(lines, fmt.Sprintf("Success Rate: %.1f%%", summary.SuccessRate*100))
	lines = append(lines, fmt.Sprintf("Total Duration: %dms", summary.TotalDurationMS))

	// Trust breakdown
	if len(summary.TrustDistribution) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Trust Breakdown:")
		for trustClass, count := range summary.TrustDistribution {
			lines = append(lines, fmt.Sprintf("  - %s: %d", trustClass, count))
		}
	}

	// Risk assessment
	if len(summary.HighRiskExecutions) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("High-Risk Executions: %d", len(summary.HighRiskExecutions)))
		for _, risk := range summary.HighRiskExecutions {
			lines = append(lines, fmt.Sprintf("  - %s (%s): %v", risk.CapabilityName, risk.TrustClass, risk.EffectClasses))
		}
	}

	// Policy violations
	if len(summary.PolicyViolations) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Policy Violations: %d", len(summary.PolicyViolations)))
		for _, violation := range summary.PolicyViolations {
			lines = append(lines, fmt.Sprintf("  - %s (%s): %s", violation.CapabilityName, violation.TrustClass, violation.ViolationType))
		}
	}

	// Insertion distribution
	if len(summary.InsertionDistribution) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Insertion Policy:")
		for action, count := range summary.InsertionDistribution {
			lines = append(lines, fmt.Sprintf("  - %s: %d", action, count))
		}
	}

	return strings.Join(lines, "\n")
}

// ToJSON serializes the provenance summary to JSON.
func (s *ProvenanceSummary) ToJSON() (string, error) {
	jsonBytes, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal provenance: %w", err)
	}
	return string(jsonBytes), nil
}

// FromJSON deserializes a provenance summary from JSON.
func ProvenanceSummaryFromJSON(jsonStr string) (*ProvenanceSummary, error) {
	var summary ProvenanceSummary
	if err := json.Unmarshal([]byte(jsonStr), &summary); err != nil {
		return nil, fmt.Errorf("unmarshal provenance: %w", err)
	}
	return &summary, nil
}

// deduplicate removes duplicate strings from a slice while preserving order.
func deduplicate(items []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(items))
	for _, item := range items {
		if _, exists := seen[item]; !exists {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

// formatTime returns the current time as an RFC3339 string.
func formatTime() string {
	return time.Now().UTC().Format(time.RFC3339)
}
