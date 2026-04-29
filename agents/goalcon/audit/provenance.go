package audit

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/agents/plan"
)

// ProvenanceSummary summarizes plan execution provenance.
type ProvenanceSummary struct {
	PlanID                     string  `json:"plan_id"`
	PlanGoal                   string  `json:"plan_goal"`
	TotalCapabilityInvocations int     `json:"total_capability_invocations"`
	UniqueCapabilities         int     `json:"unique_capabilities"`
	SuccessfulInvocations      int     `json:"successful_invocations"`
	FailedInvocations          int     `json:"failed_invocations"`
	SuccessRate                float64 `json:"success_rate"`
	HumanSummary               string  `json:"human_summary"`
}

// ProvenanceCollector builds provenance summaries from an audit trail.
type ProvenanceCollector struct {
	plan  *plan.Plan
	trail *CapabilityAuditTrail
}

// NewProvenanceCollector creates a collector for a plan execution.
func NewProvenanceCollector(plan *plan.Plan, _ any, trail *CapabilityAuditTrail) *ProvenanceCollector {
	return &ProvenanceCollector{plan: plan, trail: trail}
}

// BuildProvenance summarizes the recorded capability usage.
func (c *ProvenanceCollector) BuildProvenance() *ProvenanceSummary {
	if c == nil {
		return &ProvenanceSummary{}
	}

	var trailSummary AuditSummary
	if c.trail != nil {
		trailSummary = c.trail.Summary()
	}

	planID := ""
	planGoal := ""
	plannedSteps := 0
	if c.plan != nil {
		planID = c.plan.ID
		planGoal = c.plan.Goal
		plannedSteps = len(c.plan.Steps)
	}

	successRate := 0.0
	if trailSummary.TotalInvocations > 0 {
		successRate = float64(trailSummary.SuccessfulCount) / float64(trailSummary.TotalInvocations)
	}

	humanSummary := fmt.Sprintf(
		"plan %q executed %d capability invocations across %d planned steps",
		planGoal,
		trailSummary.TotalInvocations,
		plannedSteps,
	)
	if trailSummary.TotalInvocations == 0 {
		humanSummary = strings.TrimSpace(humanSummary + " with no recorded invocations")
	}

	return &ProvenanceSummary{
		PlanID:                     planID,
		PlanGoal:                   planGoal,
		TotalCapabilityInvocations: trailSummary.TotalInvocations,
		UniqueCapabilities:         trailSummary.UniqueCapabilities,
		SuccessfulInvocations:      trailSummary.SuccessfulCount,
		FailedInvocations:          trailSummary.FailedCount,
		SuccessRate:                successRate,
		HumanSummary:               humanSummary,
	}
}
