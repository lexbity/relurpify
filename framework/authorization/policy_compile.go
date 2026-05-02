package authorization

import (
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
)

const (
	policyPriorityGlobal   = 100
	policyPriorityTool     = 300
	policyPrioritySession  = 400
	policyPriorityProvider = 500
)

// CompileManifestPolicyRules compiles manifest policy surfaces into normalized policy rules.
func CompileManifestPolicyRules(m *manifest.AgentManifest) ([]core.PolicyRule, error) {
	if m == nil {
		return nil, nil
	}
	return CompileAgentSpecPolicyRules(m.Spec.Agent)
}

// CompileAgentSpecPolicyRules compiles policy surfaces from an effective agent
// spec rather than a raw manifest.
func CompileAgentSpecPolicyRules(spec *agentspec.AgentRuntimeSpec) ([]core.PolicyRule, error) {
	if spec == nil {
		return nil, nil
	}
	var rules []core.PolicyRule

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

func compileToolExecutionPolicy(toolName string, policy agentspec.ToolPolicy) (core.PolicyRule, bool) {
	if strings.TrimSpace(toolName) == "" || policy.Execute == "" {
		return core.PolicyRule{}, false
	}
	return core.PolicyRule{
		ID:       "tool:" + toolName,
		Name:     "Tool policy for " + toolName,
		Priority: policyPriorityTool,
		Enabled:  true,
		Conditions: core.PolicyConditions{
			Capabilities:    []string{toolName},
			CapabilityKinds: []core.CapabilityKind{core.CapabilityKindTool},
			RuntimeFamilies: []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyLocalTool},
		},
		Effect: permissionLevelToEffect(policy.Execute, ""),
	}, true
}

func compileCapabilityPolicy(index int, policy agentspec.CapabilityPolicy) (core.PolicyRule, error) {
	conditions, err := compileCapabilitySelector(policy.Selector)
	if err != nil {
		return core.PolicyRule{}, fmt.Errorf("capability_policies[%d] unsupported selector: %w", index, err)
	}
	return core.PolicyRule{
		ID:         fmt.Sprintf("capability-policy:%d", index),
		Name:       fmt.Sprintf("Capability policy %d", index),
		Priority:   policyPriorityTool + index,
		Enabled:    true,
		Conditions: conditions,
		Effect:     permissionLevelToEffect(policy.Execute, ""),
	}, nil
}

func compileProviderPolicy(providerID string, policy agentspec.ProviderPolicy) (core.PolicyRule, bool) {
	if strings.TrimSpace(providerID) == "" || policy.Activate == "" {
		return core.PolicyRule{}, false
	}
	return core.PolicyRule{
		ID:       "provider:" + providerID + ":activate",
		Name:     "Provider activation policy for " + providerID,
		Priority: policyPriorityProvider,
		Enabled:  true,
		Conditions: core.PolicyConditions{
			Capabilities: []string{"provider:" + providerID + ":activate"},
		},
		Effect: permissionLevelToEffect(policy.Activate, ""),
	}, true
}

func compileSessionPolicy(index int, policy agentspec.SessionPolicy) (core.PolicyRule, error) {
	corePolicy := core.SessionPolicy{
		ID:       policy.ID,
		Name:     policy.Name,
		Priority: policy.Priority,
		Enabled:  policy.Enabled,
		Selector: core.SessionSelector{
			Partitions:                append([]string{}, policy.Selector.Partitions...),
			ChannelIDs:                append([]string{}, policy.Selector.ChannelIDs...),
			Scopes:                    convertSessionScopes(policy.Selector.Scopes),
			TrustClasses:              append([]core.TrustClass{}, policy.Selector.TrustClasses...),
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
	if err := core.ValidateSessionPolicy(corePolicy); err != nil {
		return core.PolicyRule{}, err
	}
	conditions := core.PolicyConditions{
		TrustClasses:              append([]core.TrustClass{}, corePolicy.Selector.TrustClasses...),
		Partitions:                append([]string{}, corePolicy.Selector.Partitions...),
		ChannelIDs:                append([]string{}, corePolicy.Selector.ChannelIDs...),
		SessionScopes:             append([]core.SessionScope{}, corePolicy.Selector.Scopes...),
		SessionOperations:         append([]core.SessionOperation{}, corePolicy.Selector.Operations...),
		ExternalProviders:         append([]string{}, corePolicy.Selector.ExternalProviders...),
		RequireOwnership:          corePolicy.Selector.RequireOwnership,
		RequireDelegation:         corePolicy.Selector.RequireDelegation,
		RequireExternalBinding:    corePolicy.Selector.RequireExternalBinding,
		RequireResolvedExternal:   corePolicy.Selector.RequireResolvedExternal,
		RequireRestrictedExternal: corePolicy.Selector.RequireRestrictedExternal,
	}
	if len(corePolicy.Selector.ActorKinds) > 0 || len(corePolicy.Selector.ActorIDs) > 0 || corePolicy.Selector.AuthenticatedOnly != nil {
		match := core.ActorMatch{
			IDs: append([]string{}, corePolicy.Selector.ActorIDs...),
		}
		if len(corePolicy.Selector.ActorKinds) > 0 {
			match.Kind = corePolicy.Selector.ActorKinds[0]
		}
		if corePolicy.Selector.AuthenticatedOnly != nil {
			match.Authenticated = *corePolicy.Selector.AuthenticatedOnly
		}
		conditions.Actors = []core.ActorMatch{match}
	}
	return core.PolicyRule{
		ID:         corePolicy.ID,
		Name:       corePolicy.Name,
		Priority:   policyPrioritySession + corePolicy.Priority,
		Enabled:    corePolicy.Enabled,
		Conditions: conditions,
		Effect: core.PolicyEffect{
			Action:      permissionLevelToAction(corePolicy.Effect),
			Approvers:   append([]string{}, corePolicy.Approvers...),
			ApprovalTTL: corePolicy.ApprovalTTL,
			Reason:      corePolicy.Reason,
		},
	}, nil
}

func convertSessionScopes(values []agentspec.SessionScope) []core.SessionScope {
	out := make([]core.SessionScope, 0, len(values))
	for _, value := range values {
		out = append(out, core.SessionScope(value))
	}
	return out
}

func convertSessionOperations(values []agentspec.SessionOperation) []core.SessionOperation {
	out := make([]core.SessionOperation, 0, len(values))
	for _, value := range values {
		out = append(out, core.SessionOperation(value))
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

func compileGlobalPolicy(key string, level agentspec.AgentPermissionLevel) (*core.PolicyRule, error) {
	key = strings.TrimSpace(strings.ToLower(key))
	if key == "" || key == "default_tool_policy" || level == "" {
		return nil, nil
	}
	rule := &core.PolicyRule{
		ID:       "global:" + key,
		Name:     "Global policy for " + key,
		Priority: policyPriorityGlobal,
		Enabled:  true,
		Effect:   permissionLevelToEffect(level, ""),
	}
	switch key {
	case string(core.TrustClassBuiltinTrusted), string(core.TrustClassWorkspaceTrusted), string(core.TrustClassProviderLocalUntrusted), string(core.TrustClassRemoteDeclared), string(core.TrustClassRemoteApproved):
		rule.Conditions.TrustClasses = []core.TrustClass{core.TrustClass(key)}
	case string(core.RiskClassReadOnly), string(core.RiskClassDestructive), string(core.RiskClassExecute), string(core.RiskClassNetwork), string(core.RiskClassCredentialed), string(core.RiskClassExfiltration), string(core.RiskClassSessioned):
		rule.Conditions.MinRiskClasses = []core.RiskClass{core.RiskClass(key)}
	case string(core.CapabilityRuntimeFamilyLocalTool), string(core.CapabilityRuntimeFamilyProvider), string(core.CapabilityRuntimeFamilyRelurpic):
		rule.Conditions.RuntimeFamilies = []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamily(key)}
	case string(core.EffectClassFilesystemMutation), string(core.EffectClassProcessSpawn), string(core.EffectClassNetworkEgress), string(core.EffectClassCredentialUse), string(core.EffectClassExternalState), string(core.EffectClassSessionCreation), string(core.EffectClassContextInsertion):
		rule.Conditions.EffectClasses = []core.EffectClass{core.EffectClass(key)}
	default:
		return nil, fmt.Errorf("unsupported global policy class %q", key)
	}
	return rule, nil
}

func compileCapabilitySelector(selector agentspec.CapabilitySelector) (core.PolicyConditions, error) {
	legacy := selector
	if len(selector.ExcludeTags) > 0 || len(selector.Tags) > 0 || len(selector.SourceScopes) > 0 || len(selector.CoordinationRoles) > 0 ||
		len(selector.CoordinationTaskTypes) > 0 || len(selector.CoordinationExecutionModes) > 0 ||
		selector.CoordinationLongRunning != nil || selector.CoordinationDirectInsertion != nil {
		return core.PolicyConditions{}, fmt.Errorf("selector fields require descriptor-time evaluation")
	}
	conditions := core.PolicyConditions{
		TrustClasses:    append([]core.TrustClass{}, legacy.TrustClasses...),
		MinRiskClasses:  append([]core.RiskClass{}, legacy.RiskClasses...),
		RuntimeFamilies: append([]core.CapabilityRuntimeFamily{}, legacy.RuntimeFamilies...),
		EffectClasses:   append([]core.EffectClass{}, legacy.EffectClasses...),
	}
	if legacy.ID != "" {
		conditions.Capabilities = append(conditions.Capabilities, legacy.ID)
	}
	if legacy.Name != "" {
		conditions.Capabilities = append(conditions.Capabilities, legacy.Name)
	}
	if legacy.Kind != "" {
		conditions.CapabilityKinds = []core.CapabilityKind{legacy.Kind}
	}
	return conditions, nil
}

func permissionLevelToEffect(level agentspec.AgentPermissionLevel, reason string) core.PolicyEffect {
	return core.PolicyEffect{
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
