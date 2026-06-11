package authorization

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/governance/classification"
	pol "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/governance/risk"
)

const (
	policyPriorityGlobal   = 100
	policyPriorityTool     = 300
	policyPrioritySession  = 400
	policyPriorityProvider = 500
)

// CompileAgentSpecPolicyRules compiles policy rules from a governance-owned
// PolicyInput interface. The caller (capability/execution) is responsible for
// adapting its agent spec to satisfy PolicyInput before passing it in.
func CompileAgentSpecPolicyRules(spec ports.PolicyInput) ([]pol.PolicyRule, error) {
	if spec == nil {
		return nil, nil
	}
	var rules []pol.PolicyRule

	for toolName, policy := range spec.GetToolExecutionPolicy() {
		rule, ok := compileToolExecutionPolicy(toolName, policy)
		if !ok {
			continue
		}
		rules = append(rules, rule)
	}
	for i, policy := range spec.GetCapabilityPolicies() {
		rule, err := compileCapabilityPolicy(i, policy)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	for providerID, policy := range spec.GetProviderPolicies() {
		rule, ok := compileProviderPolicy(providerID, policy)
		if !ok {
			continue
		}
		rules = append(rules, rule)
	}
	for i, policy := range spec.GetSessionPolicies() {
		rule, err := compileSessionPolicy(i, policy)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	for key, level := range spec.GetGlobalPolicies() {
		rule, err := compileGlobalPolicy(key, level)
		if err != nil {
			return nil, err
		}
		if rule != nil {
			rules = append(rules, *rule)
		}
	}

	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority == rules[j].Priority {
			return strings.Compare(rules[i].ID, rules[j].ID) < 0
		}
		return rules[i].Priority > rules[j].Priority
	})
	return rules, nil
}

func compileToolExecutionPolicy(toolName string, policy ports.ToolPolicyView) (pol.PolicyRule, bool) {
	if strings.TrimSpace(toolName) == "" || policy.Execute == "" {
		return pol.PolicyRule{}, false
	}
	return pol.PolicyRule{
		ID:       "tool:" + toolName,
		Name:     "Tool policy for " + toolName,
		Priority: policyPriorityTool,
		Enabled:  true,
		Conditions: pol.PolicyConditions{
			Capabilities:    []string{toolName},
			CapabilityKinds: []string{"tool"},
			RuntimeFamilies: []string{"local-tool"},
		},
		Effect: permissionLevelToEffect(policy.Execute, ""),
	}, true
}

func compileCapabilityPolicy(index int, policy ports.CapabilityPolicyView) (pol.PolicyRule, error) {
	conditions, err := compileCapabilitySelector(policy.Selector)
	if err != nil {
		return pol.PolicyRule{}, fmt.Errorf("capability_policies[%d] unsupported selector: %w", index, err)
	}
	return pol.PolicyRule{
		ID:         fmt.Sprintf("capability-policy:%d", index),
		Name:       fmt.Sprintf("Capability policy %d", index),
		Priority:   policyPriorityTool + index,
		Enabled:    true,
		Conditions: conditions,
		Effect:     permissionLevelToEffect(policy.Execute, ""),
	}, nil
}

func compileProviderPolicy(providerID string, policy ports.ProviderPolicyView) (pol.PolicyRule, bool) {
	if strings.TrimSpace(providerID) == "" || policy.Activate == "" {
		return pol.PolicyRule{}, false
	}
	return pol.PolicyRule{
		ID:       "provider:" + providerID + ":activate",
		Name:     "Provider activation policy for " + providerID,
		Priority: policyPriorityProvider,
		Enabled:  true,
		Conditions: pol.PolicyConditions{
			Capabilities: []string{"provider:" + providerID + ":activate"},
		},
		Effect: permissionLevelToEffect(policy.Activate, ""),
	}, true
}

func compileSessionPolicy(index int, policy ports.SessionPolicyView) (pol.PolicyRule, error) {
	corePolicy := pol.SessionPolicy{
		ID:       policy.ID,
		Name:     policy.Name,
		Priority: policy.Priority,
		Enabled:  policy.Enabled,
		Selector: pol.SessionSelector{
			Partitions:                append([]string{}, policy.Selector.Partitions...),
			ChannelIDs:                append([]string{}, policy.Selector.ChannelIDs...),
			Scopes:                    convertSessionScopes(policy.Selector.Scopes),
			TrustClasses:              append([]string{}, policy.Selector.TrustClasses...),
			Operations:                convertSessionOperations(policy.Selector.Operations),
			ActorKinds:                append([]string{}, policy.Selector.ActorKinds...),
			ActorIDs:                  append([]string{}, policy.Selector.ActorIDs...),
			ExternalProviders:         append([]string{}, policy.Selector.ExternalProvider...),
			RequireOwnership:          policy.Selector.RequireOwnership,
			RequireDelegation:         policy.Selector.RequireDelegation,
			RequireExternalBinding:    policy.Selector.RequireExternalBinding,
			RequireResolvedExternal:   policy.Selector.RequireResolvedExternal,
			RequireRestrictedExternal: policy.Selector.RequireRestrictedExternal,
			AuthenticatedOnly:         policy.Selector.AuthOnly,
		},
		Effect:      policy.Effect,
		Approvers:   append([]string{}, policy.Approvers...),
		ApprovalTTL: policy.ApprovalTTL,
		Reason:      policy.Reason,
	}
	if err := pol.ValidateSessionPolicy(corePolicy); err != nil {
		return pol.PolicyRule{}, err
	}
	conditions := pol.PolicyConditions{
		TrustClasses:              append([]string{}, corePolicy.Selector.TrustClasses...),
		Partitions:                append([]string{}, corePolicy.Selector.Partitions...),
		ChannelIDs:                append([]string{}, corePolicy.Selector.ChannelIDs...),
		SessionScopes:             append([]pol.SessionScope{}, corePolicy.Selector.Scopes...),
		SessionOperations:         append([]pol.SessionOperation{}, corePolicy.Selector.Operations...),
		ExternalProviders:         append([]string{}, corePolicy.Selector.ExternalProviders...),
		RequireOwnership:          corePolicy.Selector.RequireOwnership,
		RequireDelegation:         corePolicy.Selector.RequireDelegation,
		RequireExternalBinding:    corePolicy.Selector.RequireExternalBinding,
		RequireResolvedExternal:   corePolicy.Selector.RequireResolvedExternal,
		RequireRestrictedExternal: corePolicy.Selector.RequireRestrictedExternal,
	}
	if len(corePolicy.Selector.ActorKinds) > 0 || len(corePolicy.Selector.ActorIDs) > 0 || corePolicy.Selector.AuthenticatedOnly != nil {
		match := pol.ActorMatch{
			IDs: append([]string{}, corePolicy.Selector.ActorIDs...),
		}
		if len(corePolicy.Selector.ActorKinds) > 0 {
			match.Kind = corePolicy.Selector.ActorKinds[0]
		}
		if corePolicy.Selector.AuthenticatedOnly != nil {
			match.Authenticated = *corePolicy.Selector.AuthenticatedOnly
		}
		conditions.Actors = []pol.ActorMatch{match}
	}
	return pol.PolicyRule{
		ID:         corePolicy.ID,
		Name:       corePolicy.Name,
		Priority:   policyPrioritySession + corePolicy.Priority,
		Enabled:    corePolicy.Enabled,
		Conditions: conditions,
		Effect: pol.PolicyEffect{
			Action:      permissionLevelToAction(corePolicy.Effect),
			Approvers:   append([]string{}, corePolicy.Approvers...),
			ApprovalTTL: corePolicy.ApprovalTTL,
			Reason:      corePolicy.Reason,
		},
	}, nil
}

func convertSessionScopes(values []string) []pol.SessionScope {
	out := make([]pol.SessionScope, 0, len(values))
	for _, value := range values {
		out = append(out, pol.SessionScope(value))
	}
	return out
}

func convertSessionOperations(values []string) []pol.SessionOperation {
	out := make([]pol.SessionOperation, 0, len(values))
	for _, value := range values {
		out = append(out, pol.SessionOperation(value))
	}
	return out
}

func convertExternalProvidersToStrings(values []string) []string {
	out := make([]string, 0, len(values))
	out = append(out, values...)
	return out
}

func compileGlobalPolicy(key string, level string) (*pol.PolicyRule, error) {
	key = strings.TrimSpace(strings.ToLower(key))
	if key == "" || key == "default_tool_policy" || level == "" {
		return nil, errors.New("invalid global policy key or level")
	}
	rule := &pol.PolicyRule{
		ID:       "global:" + key,
		Name:     "Global policy for " + key,
		Priority: policyPriorityGlobal,
		Enabled:  true,
		Effect:   permissionLevelToEffect(level, ""),
	}
	switch key {
	case "builtin-trusted", "workspace-trusted", "provider-local-untrusted", "remote-declared-untrusted", "remote-approved":
		rule.Conditions.TrustClasses = []string{key}
	case string(risk.RiskClassReadOnly), string(risk.RiskClassDestructive), string(risk.RiskClassExecute), string(risk.RiskClassNetwork), string(risk.RiskClassCredentialed), string(risk.RiskClassExfiltration), string(risk.RiskClassSessioned):
		rule.Conditions.MinRiskClasses = []risk.RiskClass{risk.RiskClass(key)}
	case "local-tool", "provider", "relurpic":
		rule.Conditions.RuntimeFamilies = []string{key}
	case string(classification.EffectClassFilesystemMutation), string(classification.EffectClassProcessSpawn), string(classification.EffectClassNetworkEgress), string(classification.EffectClassCredentialUse), string(classification.EffectClassExternalState), string(classification.EffectClassSessionCreation), string(classification.EffectClassContextInsertion):
		rule.Conditions.EffectClasses = []classification.EffectClass{classification.EffectClass(key)}
	default:
		return nil, fmt.Errorf("unsupported global policy class %q", key)
	}
	return rule, nil
}

func compileCapabilitySelector(selector ports.CapabilitySelectorView) (pol.PolicyConditions, error) {
	legacy := selector
	if len(selector.ExcludeTags) > 0 || len(selector.Tags) > 0 || len(selector.SourceScopes) > 0 || len(selector.CoordinationRoles) > 0 ||
		len(selector.CoordinationTaskTypes) > 0 || len(selector.CoordinationExecModes) > 0 ||
		selector.CoordinationLongRunning != 0 || selector.CoordinationDirectInsertion != 0 {
		return pol.PolicyConditions{}, fmt.Errorf("selector fields require descriptor-time evaluation")
	}
	conditions := pol.PolicyConditions{
		TrustClasses:    append([]string{}, legacy.TrustClasses...),
		MinRiskClasses:  append([]risk.RiskClass{}, toRiskClasses(legacy.RiskClasses)...),
		RuntimeFamilies: append([]string{}, legacy.RuntimeFamilies...),
		EffectClasses:   append([]classification.EffectClass{}, toEffectClasses(legacy.EffectClasses)...),
	}
	if legacy.ID != "" {
		conditions.Capabilities = append(conditions.Capabilities, legacy.ID)
	}
	if legacy.Name != "" {
		conditions.Capabilities = append(conditions.Capabilities, legacy.Name)
	}
	if legacy.Kind != "" {
		conditions.CapabilityKinds = []string{legacy.Kind}
	}
	return conditions, nil
}

func toRiskClasses(values []string) []risk.RiskClass {
	out := make([]risk.RiskClass, 0, len(values))
	for _, v := range values {
		out = append(out, risk.RiskClass(v))
	}
	return out
}

func toEffectClasses(values []string) []classification.EffectClass {
	out := make([]classification.EffectClass, 0, len(values))
	for _, v := range values {
		out = append(out, classification.EffectClass(v))
	}
	return out
}

func permissionLevelToEffect(level string, reason string) pol.PolicyEffect {
	return pol.PolicyEffect{
		Action: permissionLevelToAction(level),
		Reason: reason,
	}
}

func permissionLevelToAction(level string) string {
	switch level {
	case "allow":
		return "allow"
	case "deny":
		return "deny"
	default:
		return "require_approval"
	}
}
