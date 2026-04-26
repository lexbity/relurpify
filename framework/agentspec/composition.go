package agentspec

import (
	"fmt"
	"strings"
)

type MemoryMode string

const (
	MemoryModeFresh  MemoryMode = "fresh"
	MemoryModeShared MemoryMode = "shared"
	MemoryModeCloned MemoryMode = "cloned"
)

type StateMode string

const (
	StateModeFresh  StateMode = "fresh"
	StateModeShared StateMode = "shared"
	StateModeCloned StateMode = "cloned"
	StateModeForked StateMode = "forked"
)

type ToolScopePolicy string

const (
	ToolScopeInherits ToolScopePolicy = "inherits"
	ToolScopeScoped   ToolScopePolicy = "scoped"
	ToolScopeCustom   ToolScopePolicy = "custom"
)

type AgentInvocationPolicy struct {
	MemoryMode MemoryMode      `yaml:"memory_mode,omitempty" json:"memory_mode,omitempty"`
	StateMode  StateMode       `yaml:"state_mode,omitempty" json:"state_mode,omitempty"`
	ToolScope  ToolScopePolicy `yaml:"tool_scope,omitempty" json:"tool_scope,omitempty"`
}

func (p AgentInvocationPolicy) Validate() error {
	switch p.MemoryMode {
	case "", MemoryModeFresh, MemoryModeShared, MemoryModeCloned:
	default:
		return fmt.Errorf("memory_mode %q invalid", p.MemoryMode)
	}
	switch p.StateMode {
	case "", StateModeFresh, StateModeShared, StateModeCloned, StateModeForked:
	default:
		return fmt.Errorf("state_mode %q invalid", p.StateMode)
	}
	switch p.ToolScope {
	case "", ToolScopeInherits, ToolScopeScoped, ToolScopeCustom:
	default:
		return fmt.Errorf("tool_scope %q invalid", p.ToolScope)
	}
	return nil
}

type AgentCompositionSpec struct {
	Type    string                 `yaml:"type,omitempty" json:"type,omitempty"`
	Handler string                 `yaml:"handler,omitempty" json:"handler,omitempty"`
	Policy  *AgentInvocationPolicy `yaml:"policy,omitempty" json:"policy,omitempty"`
}

func (s *AgentCompositionSpec) Validate() error {
	if s == nil {
		return nil
	}
	if strings.TrimSpace(s.Type) == "" {
		return fmt.Errorf("composition type required")
	}
	if s.Type == "custom" && strings.TrimSpace(s.Handler) == "" {
		return fmt.Errorf("composition handler required for custom type")
	}
	if s.Policy != nil {
		if err := s.Policy.Validate(); err != nil {
			return fmt.Errorf("composition policy invalid: %w", err)
		}
	}
	return nil
}

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
