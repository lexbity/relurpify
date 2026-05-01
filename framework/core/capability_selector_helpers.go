package core

import (
	"fmt"
	"strings"
)

// CloneCapabilitySelectors returns a deep copy of selector slices so callers
// can safely retain or mutate them without aliasing the source.
func CloneCapabilitySelectors(selectors []CapabilitySelector) []CapabilitySelector {
	if selectors == nil {
		return nil
	}
	out := make([]CapabilitySelector, len(selectors))
	for i, selector := range selectors {
		out[i] = CloneCapabilitySelector(selector)
	}
	return out
}

// CloneCapabilitySelector returns a deep copy of one selector.
func CloneCapabilitySelector(selector CapabilitySelector) CapabilitySelector {
	if selector.RuntimeFamilies != nil {
		selector.RuntimeFamilies = append([]CapabilityRuntimeFamily{}, selector.RuntimeFamilies...)
	}
	if selector.Tags != nil {
		selector.Tags = append([]string{}, selector.Tags...)
	}
	if selector.ExcludeTags != nil {
		selector.ExcludeTags = append([]string{}, selector.ExcludeTags...)
	}
	if selector.SourceScopes != nil {
		selector.SourceScopes = append([]CapabilityScope{}, selector.SourceScopes...)
	}
	if selector.TrustClasses != nil {
		selector.TrustClasses = append([]TrustClass{}, selector.TrustClasses...)
	}
	if selector.RiskClasses != nil {
		selector.RiskClasses = append([]RiskClass{}, selector.RiskClasses...)
	}
	if selector.EffectClasses != nil {
		selector.EffectClasses = append([]EffectClass{}, selector.EffectClasses...)
	}
	if selector.CoordinationRoles != nil {
		selector.CoordinationRoles = append([]CoordinationRole{}, selector.CoordinationRoles...)
	}
	if selector.CoordinationTaskTypes != nil {
		selector.CoordinationTaskTypes = append([]string{}, selector.CoordinationTaskTypes...)
	}
	if selector.CoordinationExecutionModes != nil {
		selector.CoordinationExecutionModes = append([]CoordinationExecutionMode{}, selector.CoordinationExecutionModes...)
	}
	if selector.CoordinationLongRunning != nil {
		value := *selector.CoordinationLongRunning
		selector.CoordinationLongRunning = &value
	}
	if selector.CoordinationDirectInsertion != nil {
		value := *selector.CoordinationDirectInsertion
		selector.CoordinationDirectInsertion = &value
	}
	return selector
}

// MergeCapabilitySelectors appends selectors and deduplicates by semantic
// selector key while preserving first-seen order.
func MergeCapabilitySelectors(base, extra []CapabilitySelector) []CapabilitySelector {
	if len(extra) == 0 {
		return CloneCapabilitySelectors(base)
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]CapabilitySelector, 0, len(base)+len(extra))
	for _, selector := range append(append([]CapabilitySelector{}, base...), extra...) {
		key := capabilitySelectorKey(selector)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, CloneCapabilitySelector(selector))
	}
	return out
}

func capabilitySelectorKey(selector CapabilitySelector) string {
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
		boolPointerKey(selector.CoordinationLongRunning) + "|" +
		boolPointerKey(selector.CoordinationDirectInsertion)
}

func runtimeFamiliesToStrings(values []CapabilityRuntimeFamily) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func capabilityScopesToStrings(values []CapabilityScope) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func trustClassesToStrings(values []TrustClass) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func riskClassesToStrings(values []RiskClass) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func effectClassesToStrings(values []EffectClass) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func coordinationRolesToStrings(values []CoordinationRole) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func coordinationExecutionModesToStrings(values []CoordinationExecutionMode) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func boolPointerKey(value *bool) string {
	if value == nil {
		return ""
	}
	if *value {
		return "true"
	}
	return "false"
}

// ValidateCapabilitySelector checks the legacy selector for obvious structural
// issues. The broader matching rules are handled elsewhere in the framework.
func ValidateCapabilitySelector(selector CapabilitySelector) error {
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
		selector.CoordinationLongRunning == nil &&
		selector.CoordinationDirectInsertion == nil {
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
		case CapabilityScopeBuiltin, CapabilityScopeWorkspace, CapabilityScopeProvider, CapabilityScopeRemote:
		default:
			return fmt.Errorf("source scope %s invalid", scope)
		}
	}
	for _, family := range selector.RuntimeFamilies {
		switch family {
		case CapabilityRuntimeFamilyLocalTool, CapabilityRuntimeFamilyProvider, CapabilityRuntimeFamilyRelurpic:
		default:
			return fmt.Errorf("runtime family %s invalid", family)
		}
	}
	for _, trust := range selector.TrustClasses {
		switch trust {
		case TrustClassBuiltinTrusted, TrustClassWorkspaceTrusted, TrustClassLLMGenerated, TrustClassToolResult, TrustClassProviderLocalUntrusted, TrustClassRemoteDeclared, TrustClassRemoteApproved:
		default:
			return fmt.Errorf("trust class %s invalid", trust)
		}
	}
	for _, risk := range selector.RiskClasses {
		switch risk {
		case RiskClassReadOnly, RiskClassDestructive, RiskClassExecute, RiskClassNetwork, RiskClassCredentialed, RiskClassExfiltration, RiskClassSessioned:
		default:
			return fmt.Errorf("risk class %s invalid", risk)
		}
	}
	for _, effect := range selector.EffectClasses {
		switch effect {
		case EffectClassFilesystemMutation, EffectClassProcessSpawn, EffectClassNetworkEgress, EffectClassCredentialUse, EffectClassExternalState, EffectClassSessionCreation, EffectClassContextInsertion:
		default:
			return fmt.Errorf("effect class %s invalid", effect)
		}
	}
	for _, role := range selector.CoordinationRoles {
		switch role {
		case CoordinationRolePlanner, CoordinationRoleArchitect, CoordinationRoleReviewer, CoordinationRoleVerifier, CoordinationRoleExecutor, CoordinationRoleDomainPack, CoordinationRoleBackgroundAgent:
		default:
			return fmt.Errorf("coordination role %s invalid", role)
		}
	}
	for _, mode := range selector.CoordinationExecutionModes {
		switch mode {
		case CoordinationExecutionModeSync, CoordinationExecutionModeSessionBacked, CoordinationExecutionModeBackgroundAgent:
		default:
			return fmt.Errorf("coordination execution mode %s invalid", mode)
		}
	}
	return nil
}
