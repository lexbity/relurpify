package rex

import (
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/rex/route"
)

func enforceCapabilityProjection(state *core.Context, decision route.RouteDecision, task *core.Task) error {
	projection, ok := capabilityProjectionFromState(state)
	if !ok {
		return nil
	}
	if len(projection.AllowedTaskClasses) > 0 && !containsFold(projection.AllowedTaskClasses, "agent.run") {
		return fmt.Errorf("capability projection rejects rex task class agent.run")
	}
	for _, capabilityID := range requiredCapabilities(decision, task) {
		if len(projection.AllowedCapabilityIDs) > 0 && !containsFold(projection.AllowedCapabilityIDs, capabilityID) {
			return fmt.Errorf("capability projection rejects required capability %s", capabilityID)
		}
	}
	return nil
}

func capabilityProjectionFromState(state *core.Context) (core.CapabilityEnvelope, bool) {
	if state == nil {
		return core.CapabilityEnvelope{}, false
	}
	raw, ok := state.Get("fmp.capability_projection")
	if !ok || raw == nil {
		return core.CapabilityEnvelope{}, false
	}
	if projection, ok := raw.(core.CapabilityEnvelope); ok {
		return projection, true
	}
	if projection, ok := raw.(*core.CapabilityEnvelope); ok && projection != nil {
		return *projection, true
	}
	return core.CapabilityEnvelope{}, false
}

func requiredCapabilities(decision route.RouteDecision, task *core.Task) []string {
	required := []string{string(core.CapabilityExecute)}
	if decision.Family == route.FamilyPlanner || decision.Mode == "planning" {
		required = append(required, string(core.CapabilityPlan))
	}
	if task != nil {
		switch task.Type {
		case core.TaskTypeCodeGeneration, core.TaskTypeCodeModification:
			required = append(required, string(core.CapabilityCode))
		case core.TaskTypeReview, core.TaskTypeAnalysis:
			required = append(required, string(core.CapabilityExplain))
		}
	}
	return uniqueStrings(required)
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}
