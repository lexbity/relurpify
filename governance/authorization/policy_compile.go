package authorization

import (
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	pol "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

const (
	policyPriorityGlobal   = 100
	policyPriorityTool     = 300
	policyPrioritySession  = 400
	policyPriorityProvider = 500
)

// CompileManifestPolicyRules compiles manifest policy surfaces into normalized policy rules.
func CompileManifestPolicyRules(m *config.AgentManifest) ([]pol.PolicyRule, error) {
	if m == nil {
		return nil, nil
	}
	return CompileAgentSpecPolicyRules(m.Spec.Agent)
}

// CompileAgentSpecPolicyRules compiles policy surfaces from an effective agent
// spec rather than a raw manifest.
func CompileAgentSpecPolicyRules(spec *agentspec.AgentRuntimeSpec) ([]pol.PolicyRule, error) {
	if spec == nil {
		return nil, nil
	}
	var rules []pol.PolicyRule

	for toolName, policy := range spec.ToolExecutionPolicy {
		rule, ok := compileToolExecutionPolicy(toolName, policy)
		if !ok {
			continue
		}
		rules = append(rules, rule)
	}
	for i, policy := range spec.CapabilityPolicies {
		rule, err := compileCapabilityPolicy(i, policy)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	for providerID, policy := range spec.ProviderPolicies {
		rule, ok := compileProviderPolicy(providerID, policy)
		if !ok {
			continue
		}
		rules = append(rules, rule)
	}
	for i, policy := range spec.SessionPolicies {
		rule, err := compileSessionPolicy(i, policy)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	for key, level := range spec.GlobalPolicies {
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

func compileToolExecutionPolicy(toolName string, policy agentspec.ToolPolicy) (pol.PolicyRule, bool) {
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
			CapabilityKinds: []agentspec.CapabilityKind{agentspec.CapabilityKindTool},
			RuntimeFamilies: []agentspec.CapabilityRuntimeFamily{agentspec.CapabilityRuntimeFamilyLocalTool},
		},
		Effect: permissionLevelToEffect(policy.Execute, ""),
	}, true
}

func compileCapabilityPolicy(index int, policy agentspec.CapabilityPolicy) (pol.PolicyRule, error) {
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

func compileProviderPolicy(providerID string, policy agentspec.ProviderPolicy) (pol.PolicyRule, bool) {
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

func compileSessionPolicy(index int, policy agentspec.SessionPolicy) (pol.PolicyRule, error) {
	corePolicy := pol.SessionPolicy{
		ID:       policy.ID,
		Name:     policy.Name,
		Priority: policy.Priority,
		Enabled:  policy.Enabled,
		Selector: pol.SessionSelector{
			Partitions:                append([]string{}, policy.Selector.Partitions...),
			ChannelIDs:                append([]string{}, policy.Selector.ChannelIDs...),
			Scopes:                    convertSessionScopes(policy.Selector.Scopes),
			TrustClasses:              append([]agentspec.TrustClass{}, policy.Selector.TrustClasses...),
			Operations:                convertSessionOperations(policy.Selector.Operations),
			ActorKinds:                append([]string{}, policy.Selector.ActorKinds...),
			ActorIDs:                  append([]string{}, policy.Selector.ActorIDs...),
			ExternalProviders:         convertExternalProvidersToStrings(policy.Selector.ExternalProviders),
			RequireOwnership:          policy.Selector.RequireOwnership,
			RequireDelegation:         policy.Selector.RequireDelegation,
			RequireExternalBinding:    policy.Selector.RequireExternalBinding,
			RequireResolvedExternal:   policy.Selector.RequireResolvedExternal,
			RequireRestrictedExternal: policy.Selector.RequireRestrictedExternal,
			AuthenticatedOnly:         policy.Selector.AuthenticatedOnly,
		},
		Effect:      agentspec.AgentPermissionLevel(policy.Effect),
		Approvers:   append([]string{}, policy.Approvers...),
		ApprovalTTL: policy.ApprovalTTL,
		Reason:      policy.Reason,
	}
	if err := pol.ValidateSessionPolicy(corePolicy); err != nil {
		return pol.PolicyRule{}, err
	}
	conditions := pol.PolicyConditions{
		TrustClasses:              append([]agentspec.TrustClass{}, corePolicy.Selector.TrustClasses...),
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

func convertSessionScopes(values []agentspec.SessionScope) []pol.SessionScope {
	out := make([]pol.SessionScope, 0, len(values))
	for _, value := range values {
		out = append(out, pol.SessionScope(value))
	}
	return out
}

func convertSessionOperations(values []agentspec.SessionOperation) []pol.SessionOperation {
	out := make([]pol.SessionOperation, 0, len(values))
	for _, value := range values {
		out = append(out, pol.SessionOperation(value))
	}
	return out
}

func convertExternalProvidersToStrings(values []agentspec.ExternalProvider) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func compileGlobalPolicy(key string, level agentspec.AgentPermissionLevel) (*pol.PolicyRule, error) {
	key = strings.TrimSpace(strings.ToLower(key))
	if key == "" || key == "default_tool_policy" || level == "" {
		return nil, nil
	}
	rule := &pol.PolicyRule{
		ID:       "global:" + key,
		Name:     "Global policy for " + key,
		Priority: policyPriorityGlobal,
		Enabled:  true,
		Effect:   permissionLevelToEffect(level, ""),
	}
	switch key {
	case string(agentspec.TrustClassBuiltinTrusted), string(agentspec.TrustClassWorkspaceTrusted), string(agentspec.TrustClassProviderLocalUntrusted), string(agentspec.TrustClassRemoteDeclared), string(agentspec.TrustClassRemoteApproved):
		rule.Conditions.TrustClasses = []agentspec.TrustClass{agentspec.TrustClass(key)}
	case string(agentspec.RiskClassReadOnly), string(agentspec.RiskClassDestructive), string(agentspec.RiskClassExecute), string(agentspec.RiskClassNetwork), string(agentspec.RiskClassCredentialed), string(agentspec.RiskClassExfiltration), string(agentspec.RiskClassSessioned):
		rule.Conditions.MinRiskClasses = []agentspec.RiskClass{agentspec.RiskClass(key)}
	case string(agentspec.CapabilityRuntimeFamilyLocalTool), string(agentspec.CapabilityRuntimeFamilyProvider), string(agentspec.CapabilityRuntimeFamilyRelurpic):
		rule.Conditions.RuntimeFamilies = []agentspec.CapabilityRuntimeFamily{agentspec.CapabilityRuntimeFamily(key)}
	case string(agentspec.EffectClassFilesystemMutation), string(agentspec.EffectClassProcessSpawn), string(agentspec.EffectClassNetworkEgress), string(agentspec.EffectClassCredentialUse), string(agentspec.EffectClassExternalState), string(agentspec.EffectClassSessionCreation), string(agentspec.EffectClassContextInsertion):
		rule.Conditions.EffectClasses = []agentspec.EffectClass{agentspec.EffectClass(key)}
	default:
		return nil, fmt.Errorf("unsupported global policy class %q", key)
	}
	return rule, nil
}

func compileCapabilitySelector(selector agentspec.CapabilitySelector) (pol.PolicyConditions, error) {
	legacy := selector
	if len(selector.ExcludeTags) > 0 || len(selector.Tags) > 0 || len(selector.SourceScopes) > 0 || len(selector.CoordinationRoles) > 0 ||
		len(selector.CoordinationTaskTypes) > 0 || len(selector.CoordinationExecutionModes) > 0 ||
		selector.CoordinationLongRunning != agentspec.EnabledStateUnset || selector.CoordinationDirectInsertion != agentspec.EnabledStateUnset {
		return pol.PolicyConditions{}, fmt.Errorf("selector fields require descriptor-time evaluation")
	}
	conditions := pol.PolicyConditions{
		TrustClasses:    append([]agentspec.TrustClass{}, legacy.TrustClasses...),
		MinRiskClasses:  append([]agentspec.RiskClass{}, legacy.RiskClasses...),
		RuntimeFamilies: append([]agentspec.CapabilityRuntimeFamily{}, legacy.RuntimeFamilies...),
		EffectClasses:   append([]agentspec.EffectClass{}, legacy.EffectClasses...),
	}
	if legacy.ID != "" {
		conditions.Capabilities = append(conditions.Capabilities, legacy.ID)
	}
	if legacy.Name != "" {
		conditions.Capabilities = append(conditions.Capabilities, legacy.Name)
	}
	if legacy.Kind != "" {
		conditions.CapabilityKinds = []agentspec.CapabilityKind{legacy.Kind}
	}
	return conditions, nil
}

func permissionLevelToEffect(level agentspec.AgentPermissionLevel, reason string) pol.PolicyEffect {
	return pol.PolicyEffect{
		Action: permissionLevelToAction(level),
		Reason: reason,
	}
}

func permissionLevelToAction(level agentspec.AgentPermissionLevel) string {
	switch level {
	case agentspec.AgentPermissionAllow:
		return "allow"
	case agentspec.AgentPermissionDeny:
		return "deny"
	default:
		return "require_approval"
	}
}
