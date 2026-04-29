package rex

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/rex/route"
	"codeburg.org/lexbit/relurpify/relurpnet/fmp"
)

func enforceCapabilityProjection(env *contextdata.Envelope, decision route.RouteDecision, task *core.Task) error {
	projection, ok := capabilityProjectionFromEnvelope(env)
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

func capabilityProjectionFromEnvelope(env *contextdata.Envelope) (fmp.CapabilityEnvelope, bool) {
	if env == nil {
		return fmp.CapabilityEnvelope{}, false
	}
	raw, ok := env.GetWorkingValue("fmp.capability_projection")
	if !ok || raw == nil {
		return fmp.CapabilityEnvelope{}, false
	}
	if projection, ok := raw.(fmp.CapabilityEnvelope); ok {
		return projection, true
	}
	if projection, ok := raw.(*fmp.CapabilityEnvelope); ok && projection != nil {
		return *projection, true
	}
	return fmp.CapabilityEnvelope{}, false
}

func requiredCapabilities(decision route.RouteDecision, task *core.Task) []string {
	required := []string{"execute"}
	if decision.Family == route.FamilyPlanner || decision.Mode == "planning" {
		required = append(required, "plan")
	}
	if task != nil {
		switch task.Type {
		case string(core.TaskTypeCodeGeneration), string(core.TaskTypeExecute):
			required = append(required, "code")
		case string(core.TaskTypeReview), string(core.TaskTypePlan), string(core.TaskTypeExplain):
			required = append(required, "explain")
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
