package authorization

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// PolicyEngine evaluates whether a capability invocation is permitted.
type PolicyEngine interface {
	Evaluate(ctx context.Context, req policy.PolicyRequest) (policy.PolicyDecision, error)
}

// ManifestPolicyEngine implements PolicyEngine using PermissionManager rules
// declared in an agent manifest.
type ManifestPolicyEngine struct {
	agentID string
	manager *PermissionManager
	rules   []policy.PolicyRule
}

// FromManifestWithConfig constructs a ManifestPolicyEngine for the given agent.
// agentID identifies the agent in audit logs; manager carries the declared policy.
func FromManifestWithConfig(m *config.AgentManifest, agentID string, manager *PermissionManager) (*ManifestPolicyEngine, error) {
	id := agentID
	if id == "" && m != nil {
		id = m.Metadata.Name
	}
	rules, err := CompileManifestPolicyRules(m)
	if err != nil {
		return nil, err
	}
	return &ManifestPolicyEngine{agentID: id, manager: manager, rules: rules}, nil
}

// FromAgentSpecWithConfig constructs a ManifestPolicyEngine from an effective
// runtime spec rather than a raw manifest.
func FromAgentSpecWithConfig(spec ports.SpecView, agentID string, manager *PermissionManager) (*ManifestPolicyEngine, error) {
	rules, err := CompileAgentSpecPolicyRules(spec)
	if err != nil {
		return nil, err
	}
	return &ManifestPolicyEngine{agentID: agentID, manager: manager, rules: rules}, nil
}

// Evaluate decides whether req should be allowed, denied, or routed to HITL.
//
// Trust class dispatch:
//   - BuiltinTrusted / WorkspaceTrusted → always allow (declared in manifest or built in).
//   - All remote / untrusted classes → apply the agent's configured default policy.
//     Allow → pass through; Deny → hard block; Ask (default) → require approval.
func (e *ManifestPolicyEngine) Evaluate(_ context.Context, req policy.PolicyRequest) (policy.PolicyDecision, error) {
	if e == nil {
		return policy.PolicyDecisionAllow("no policy manager"), nil
	}
	if decision := evaluateCompiledRules(e.rules, req); decision != nil {
		e.emitDecision(context.Background(), req, *decision)
		return *decision, nil
	}
	if e.manager == nil {
		return policy.PolicyDecisionAllow("no policy manager"), nil
	}
	decision := e.fallbackDecision(req)
	e.emitDecision(context.Background(), req, decision)
	return decision, nil
}

func (e *ManifestPolicyEngine) fallbackDecision(req policy.PolicyRequest) policy.PolicyDecision {
	switch req.Target {
	case policy.PolicyTargetProvider:
		return e.providerFallbackDecision(req)
	case policy.PolicyTargetSession:
		return e.sessionFallbackDecision(req)
	case policy.PolicyTargetResume:
		return e.resumeFallbackDecision(req)
	default:
		return e.capabilityFallbackDecision(req)
	}
}

func (e *ManifestPolicyEngine) sessionFallbackDecision(req policy.PolicyRequest) policy.PolicyDecision {
	if req.RestrictedExternal {
		return policy.PolicyDecisionRequireApproval(nil)
	}
	if !req.IsOwner && !req.IsDelegated {
		return policy.PolicyDecisionDeny("session access requires ownership or explicit delegation")
	}
	return e.capabilityFallbackDecision(req)
}

func (e *ManifestPolicyEngine) resumeFallbackDecision(req policy.PolicyRequest) policy.PolicyDecision {
	if !req.IsOwner && !req.IsDelegated {
		return policy.PolicyDecisionDeny("resume requires ownership or explicit delegation")
	}
	if req.RestrictedExternal {
		return policy.PolicyDecisionRequireApproval(nil)
	}
	return e.capabilityFallbackDecision(req)
}

func (e *ManifestPolicyEngine) capabilityFallbackDecision(req policy.PolicyRequest) policy.PolicyDecision {
	switch req.TrustClass {
	case "builtin-trusted", "workspace-trusted":
		return policy.PolicyDecisionAllow("workspace trusted")
	default:
		switch e.manager.DefaultPolicy() {
		case "allow":
			return policy.PolicyDecisionAllow("default policy: allow")
		case "deny":
			return policy.PolicyDecisionDeny(
				fmt.Sprintf("capability %q denied by default policy for agent %s", req.CapabilityName, e.agentID),
			)
		default:
			return policy.PolicyDecisionRequireApproval(nil)
		}
	}
}

func (e *ManifestPolicyEngine) providerFallbackDecision(req policy.PolicyRequest) policy.PolicyDecision {
	switch req.ProviderKind {
	case "builtin", "agent-runtime":
		return policy.PolicyDecisionAllow("provider kind trusted by default")
	}
	if req.ProviderOrigin == "remote" ||
		req.ProviderKind == "mcp-client" ||
		req.ProviderKind == "mcp-server" {
		return policy.PolicyDecisionRequireApproval(nil)
	}
	return policy.PolicyDecisionAllow("provider allowed by default")
}

func (e *ManifestPolicyEngine) emitDecision(ctx context.Context, req policy.PolicyRequest, decision policy.PolicyDecision) {
	if e == nil || e.manager == nil {
		return
	}
	desc := permissions.PermissionDescriptor{
		Type:     permissions.PermissionTypeCapability,
		Action:   permissionActionForRequest(req),
		Resource: permissionResourceForRequest(req),
	}
	fields := map[string]interface{}{
		"target": string(req.Target),
	}
	if decision.Rule != nil {
		fields["rule_id"] = decision.Rule.ID
		fields["rule_name"] = decision.Rule.Name
	}
	e.manager.emitPolicyDecision(ctx, desc, decision.Effect, decision.Reason, fields)
}

func permissionActionForRequest(req policy.PolicyRequest) string {
	switch {
	case req.CapabilityName != "":
		return req.CapabilityName
	case req.CapabilityID != "":
		return req.CapabilityID
	case req.Target == policy.PolicyTargetResume && req.ExportName != "":
		return "resume:" + req.ExportName
	case req.Target == policy.PolicyTargetSession:
		return "session:" + string(req.SessionOperation)
	case req.Target == policy.PolicyTargetProvider:
		return "provider"
	default:
		return "capability"
	}
}

func permissionResourceForRequest(req policy.PolicyRequest) string {
	switch {
	case req.LineageID != "":
		return req.LineageID
	case req.CapabilityID != "":
		return req.CapabilityID
	case req.SessionID != "":
		return req.SessionID
	case req.Actor.ID != "":
		return req.Actor.ID
	default:
		return ""
	}
}
