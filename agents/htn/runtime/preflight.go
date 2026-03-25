package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

// planPreflight checks plan step required capabilities against the registry.
// If the registry has no capabilities registered, the check is skipped (allows
// fallback dispatch to work without failing upfront).
func planPreflight(plan *core.Plan, registry *capability.Registry) (*graph.PreflightReport, error) {
	report := &graph.PreflightReport{GeneratedAt: time.Now().UTC()}
	if plan == nil || registry == nil {
		return report, nil
	}
	all := registry.AllCapabilities()
	if len(all) == 0 {
		// Empty registry — skip capability checks; dispatch will fall back.
		return report, nil
	}
	available := make(map[string]bool, len(all)*2)
	for _, desc := range all {
		if desc.ID != "" {
			available[desc.ID] = true
		}
		if desc.Name != "" {
			available[desc.Name] = true
		}
	}
	for _, step := range plan.Steps {
		target := capabilityTargetForStep(step)
		if target != "" && !available[target] {
			report.Issues = append(report.Issues, graph.PreflightIssue{
				NodeID:   step.ID,
				Code:     "capability_missing",
				Message:  fmt.Sprintf("required capability %q not registered", target),
				Blocking: true,
			})
		}
	}
	var err error
	if report.HasBlockingIssues() {
		err = fmt.Errorf("htn: preflight failed: missing required capabilities")
	}
	return report, err
}

// capabilityTargetForStep resolves the dispatch target for a plan step.
func capabilityTargetForStep(step core.PlanStep) string {
	if step.Params != nil {
		if raw, ok := step.Params["operator_executor"]; ok {
			var typed string
			if decodeContextValue(raw, &typed) && strings.TrimSpace(typed) != "" {
				return capabilityTargetForOperator(typed)
			}
		}
	}
	return capabilityTargetForOperator(step.Tool)
}
