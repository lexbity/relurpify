package authorization

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
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
func CompileAgentSpecPolicyRules(spec *core.AgentRuntimeSpec) ([]core.PolicyRule, error) {
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

func compileToolExecutionPolicy(toolName string, policy core.ToolPolicy) (core.PolicyRule, bool) {
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

func compileCapabilityPolicy(index int, policy core.CapabilityPolicy) (core.PolicyRule, error) {
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

func compileProviderPolicy(providerID string, policy core.ProviderPolicy) (core.PolicyRule, bool) {
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

func compileSessionPolicy(index int, policy core.SessionPolicy) (core.PolicyRule, error) {
	if err := core.ValidateSessionPolicy(policy); err != nil {
		return core.PolicyRule{}, err
	}
	conditions := core.PolicyConditions{
		TrustClasses:              append([]core.TrustClass{}, policy.Selector.TrustClasses...),
		Partitions:                append([]string{}, policy.Selector.Partitions...),
		ChannelIDs:                append([]string{}, policy.Selector.ChannelIDs...),
		SessionScopes:             append([]core.SessionScope{}, policy.Selector.Scopes...),
		SessionOperations:         append([]core.SessionOperation{}, policy.Selector.Operations...),
		ExternalProviders:         append([]core.ExternalProvider{}, policy.Selector.ExternalProviders...),
		RequireOwnership:          policy.Selector.RequireOwnership,
		RequireDelegation:         policy.Selector.RequireDelegation,
		RequireExternalBinding:    policy.Selector.RequireExternalBinding,
		RequireResolvedExternal:   policy.Selector.RequireResolvedExternal,
		RequireRestrictedExternal: policy.Selector.RequireRestrictedExternal,
	}
	if len(policy.Selector.ActorKinds) > 0 || len(policy.Selector.ActorIDs) > 0 || policy.Selector.AuthenticatedOnly != nil {
		match := core.ActorMatch{
			IDs: append([]string{}, policy.Selector.ActorIDs...),
		}
		if len(policy.Selector.ActorKinds) > 0 {
			match.Kind = policy.Selector.ActorKinds[0]
		}
		if policy.Selector.AuthenticatedOnly != nil {
			match.Authenticated = *policy.Selector.AuthenticatedOnly
		}
		conditions.Actors = []core.ActorMatch{match}
	}
	return core.PolicyRule{
		ID:         policy.ID,
		Name:       policy.Name,
		Priority:   policyPrioritySession + policy.Priority,
		Enabled:    policy.Enabled,
		Conditions: conditions,
		Effect: core.PolicyEffect{
			Action:      permissionLevelToAction(policy.Effect),
			Approvers:   append([]string{}, policy.Approvers...),
			ApprovalTTL: policy.ApprovalTTL,
			Reason:      policy.Reason,
		},
	}, nil
}

func compileGlobalPolicy(key string, level core.AgentPermissionLevel) (*core.PolicyRule, error) {
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

func compileCapabilitySelector(selector core.CapabilitySelector) (core.PolicyConditions, error) {
	if len(selector.ExcludeTags) > 0 || len(selector.Tags) > 0 || len(selector.SourceScopes) > 0 || len(selector.CoordinationRoles) > 0 ||
		len(selector.CoordinationTaskTypes) > 0 || len(selector.CoordinationExecutionModes) > 0 ||
		selector.CoordinationLongRunning != nil || selector.CoordinationDirectInsertion != nil {
		return core.PolicyConditions{}, fmt.Errorf("selector fields require descriptor-time evaluation")
	}
	conditions := core.PolicyConditions{
		TrustClasses:    append([]core.TrustClass{}, selector.TrustClasses...),
		MinRiskClasses:  append([]core.RiskClass{}, selector.RiskClasses...),
		RuntimeFamilies: append([]core.CapabilityRuntimeFamily{}, selector.RuntimeFamilies...),
		EffectClasses:   append([]core.EffectClass{}, selector.EffectClasses...),
	}
	if selector.ID != "" {
		conditions.Capabilities = append(conditions.Capabilities, selector.ID)
	}
	if selector.Name != "" {
		conditions.Capabilities = append(conditions.Capabilities, selector.Name)
	}
	if selector.Kind != "" {
		conditions.CapabilityKinds = []core.CapabilityKind{selector.Kind}
	}
	return conditions, nil
}

func permissionLevelToEffect(level core.AgentPermissionLevel, reason string) core.PolicyEffect {
	return core.PolicyEffect{
		Action: permissionLevelToAction(level),
		Reason: reason,
	}
}

func permissionLevelToAction(level core.AgentPermissionLevel) string {
	switch level {
	case core.AgentPermissionAllow:
		return "allow"
	case core.AgentPermissionDeny:
		return "deny"
	default:
		return "require_approval"
	}
}
