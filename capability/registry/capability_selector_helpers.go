package registry

import (
	"fmt"
	"strconv"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/governance/risk"
)

// CloneCapabilitySelectors returns a deep copy of selector slices so callers
// can safely retain or mutate them without aliasing the source.
func CloneCapabilitySelectors(selectors []agentspec.CapabilitySelector) []agentspec.CapabilitySelector {
	if selectors == nil {
		return nil
	}
	out := make([]agentspec.CapabilitySelector, len(selectors))
	for i, selector := range selectors {
		out[i] = CloneCapabilitySelector(selector)
	}
	return out
}

// CloneCapabilitySelector returns a deep copy of one selector.
func CloneCapabilitySelector(selector agentspec.CapabilitySelector) agentspec.CapabilitySelector {
	if selector.RuntimeFamilies != nil {
		selector.RuntimeFamilies = append([]agentspec.CapabilityRuntimeFamily{}, selector.RuntimeFamilies...)
	}
	if selector.Tags != nil {
		selector.Tags = append([]string{}, selector.Tags...)
	}
	if selector.ExcludeTags != nil {
		selector.ExcludeTags = append([]string{}, selector.ExcludeTags...)
	}
	if selector.SourceScopes != nil {
		selector.SourceScopes = append([]classification.CapabilityScope{}, selector.SourceScopes...)
	}
	if selector.TrustClasses != nil {
		selector.TrustClasses = append([]agentspec.TrustClass{}, selector.TrustClasses...)
	}
	if selector.RiskClasses != nil {
		selector.RiskClasses = append([]risk.RiskClass{}, selector.RiskClasses...)
	}
	if selector.EffectClasses != nil {
		selector.EffectClasses = append([]classification.EffectClass{}, selector.EffectClasses...)
	}
	if selector.CoordinationRoles != nil {
		selector.CoordinationRoles = append([]agentspec.CoordinationRole{}, selector.CoordinationRoles...)
	}
	if selector.CoordinationTaskTypes != nil {
		selector.CoordinationTaskTypes = append([]string{}, selector.CoordinationTaskTypes...)
	}
	if selector.CoordinationExecutionModes != nil {
		selector.CoordinationExecutionModes = append([]agentspec.CoordinationExecutionMode{}, selector.CoordinationExecutionModes...)
	}
	return selector
}

// MergeCapabilitySelectors appends selectors and deduplicates by semantic
// selector key while preserving first-seen order.
func MergeCapabilitySelectors(base, extra []agentspec.CapabilitySelector) []agentspec.CapabilitySelector {
	if len(extra) == 0 {
		return CloneCapabilitySelectors(base)
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]agentspec.CapabilitySelector, 0, len(base)+len(extra))
	for _, selector := range append(append([]agentspec.CapabilitySelector{}, base...), extra...) {
		key := capabilitySelectorKey(selector)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, CloneCapabilitySelector(selector))
	}
	return out
}

func capabilitySelectorKey(selector agentspec.CapabilitySelector) string {
	return selector.ID + "|" + selector.Name + "|" + string(selector.Kind) + "|" +
		strings.Join(runtimeFamiliesToStrings(selector.RuntimeFamilies), ",") + "|" +
		strings.Join(selector.Tags, ",") + "|" + strings.Join(selector.ExcludeTags, ",") + "|" +
		strings.Join(capabilityScopesToStrings(selector.SourceScopes), ",") + "|" +
		strings.Join(trustClassesToStrings(selector.TrustClasses), ",") + "|" +
		strings.Join(riskClassesToStrings(selector.RiskClasses), ",") + "|" +
		strings.Join(effectClassesToStrings(selector.EffectClasses), ",") + "|" +
		strings.Join(coordinationRolesToStrings(selector.CoordinationRoles), ",") + "|" +
		strings.Join(selector.CoordinationTaskTypes, ",") + "|" +
		strings.Join(coordinationExecutionModesToStrings(selector.CoordinationExecutionModes), ",") + "|" +
		enabledStateKey(selector.CoordinationLongRunning) + "|" +
		enabledStateKey(selector.CoordinationDirectInsertion)
}

func runtimeFamiliesToStrings(values []agentspec.CapabilityRuntimeFamily) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func capabilityScopesToStrings(values []classification.CapabilityScope) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func trustClassesToStrings(values []agentspec.TrustClass) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func riskClassesToStrings(values []risk.RiskClass) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func effectClassesToStrings(values []classification.EffectClass) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func coordinationRolesToStrings(values []agentspec.CoordinationRole) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func coordinationExecutionModesToStrings(values []agentspec.CoordinationExecutionMode) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func enabledStateKey(value agentspec.EnabledState) string {
	return strconv.Itoa(int(value))
}

// ValidateCapabilitySelector checks the selector for obvious structural issues.
// The broader matching rules are handled elsewhere in the framework.
func ValidateCapabilitySelector(selector agentspec.CapabilitySelector) error {
	if strings.TrimSpace(selector.ID) == "" &&
		strings.TrimSpace(selector.Name) == "" &&
		selector.Kind == "" &&
		len(selector.RuntimeFamilies) == 0 &&
		len(selector.Tags) == 0 &&
		len(selector.ExcludeTags) == 0 &&
		len(selector.SourceScopes) == 0 &&
		len(selector.TrustClasses) == 0 &&
		len(selector.RiskClasses) == 0 &&
		len(selector.EffectClasses) == 0 &&
		len(selector.CoordinationRoles) == 0 &&
		len(selector.CoordinationTaskTypes) == 0 &&
		len(selector.CoordinationExecutionModes) == 0 &&
		selector.CoordinationLongRunning == agentspec.EnabledStateUnset &&
		selector.CoordinationDirectInsertion == agentspec.EnabledStateUnset {
		return fmt.Errorf("selector must declare at least one match field")
	}
	for _, tag := range append([]string{}, selector.Tags...) {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("selector contains empty tag")
		}
	}
	for _, tag := range selector.ExcludeTags {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("selector contains empty tag")
		}
	}
	for _, taskType := range selector.CoordinationTaskTypes {
		if strings.TrimSpace(taskType) == "" {
			return fmt.Errorf("selector contains empty coordination task type")
		}
	}
	for _, scope := range selector.SourceScopes {
		switch scope {
		case classification.CapabilityScopeBuiltin, classification.CapabilityScopeWorkspace, classification.CapabilityScopeProvider, classification.CapabilityScopeRemote:
		default:
			return fmt.Errorf("source scope %s invalid", scope)
		}
	}
	for _, family := range selector.RuntimeFamilies {
		switch family {
		case agentspec.CapabilityRuntimeFamilyLocalTool, agentspec.CapabilityRuntimeFamilyProvider, agentspec.CapabilityRuntimeFamilyRelurpic:
		default:
			return fmt.Errorf("runtime family %s invalid", family)
		}
	}
	for _, trust := range selector.TrustClasses {
		switch trust {
		case agentspec.TrustClassBuiltinTrusted, agentspec.TrustClassWorkspaceTrusted, agentspec.TrustClassLLMGenerated, agentspec.TrustClassToolResult, agentspec.TrustClassProviderLocalUntrusted, agentspec.TrustClassRemoteDeclared, agentspec.TrustClassRemoteApproved:
		default:
			return fmt.Errorf("trust class %s invalid", trust)
		}
	}
	for _, rc := range selector.RiskClasses {
		switch rc {
		case risk.RiskClassReadOnly, risk.RiskClassDestructive, risk.RiskClassExecute, risk.RiskClassNetwork, risk.RiskClassCredentialed, risk.RiskClassExfiltration, risk.RiskClassSessioned:
		default:
			return fmt.Errorf("risk class %s invalid", rc)
		}
	}
	for _, effect := range selector.EffectClasses {
		switch effect {
		case classification.EffectClassFilesystemMutation, classification.EffectClassProcessSpawn, classification.EffectClassNetworkEgress, classification.EffectClassCredentialUse, classification.EffectClassExternalState, classification.EffectClassSessionCreation, classification.EffectClassContextInsertion:
		default:
			return fmt.Errorf("effect class %s invalid", effect)
		}
	}
	for _, role := range selector.CoordinationRoles {
		switch role {
		case agentspec.CoordinationRolePlanner, agentspec.CoordinationRoleArchitect, agentspec.CoordinationRoleReviewer, agentspec.CoordinationRoleVerifier, agentspec.CoordinationRoleExecutor, agentspec.CoordinationRoleDomainPack, agentspec.CoordinationRoleBackgroundAgent:
		default:
			return fmt.Errorf("coordination role %s invalid", role)
		}
	}
	for _, mode := range selector.CoordinationExecutionModes {
		switch mode {
		case agentspec.CoordinationExecutionModeSync, agentspec.CoordinationExecutionModeSessionBacked, agentspec.CoordinationExecutionModeBackgroundAgent:
		default:
			return fmt.Errorf("coordination execution mode %s invalid", mode)
		}
	}
	switch selector.CoordinationLongRunning {
	case agentspec.EnabledStateUnset, agentspec.EnabledStateEnabled, agentspec.EnabledStateDisabled:
	default:
		return fmt.Errorf("coordination long_running state %d invalid", selector.CoordinationLongRunning)
	}
	switch selector.CoordinationDirectInsertion {
	case agentspec.EnabledStateUnset, agentspec.EnabledStateEnabled, agentspec.EnabledStateDisabled:
	default:
		return fmt.Errorf("coordination direct_insertion state %d invalid", selector.CoordinationDirectInsertion)
	}
	return nil
}
