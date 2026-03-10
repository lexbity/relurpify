package authorization

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
)

// PolicyEngine evaluates whether a capability invocation is permitted.
type PolicyEngine interface {
	Evaluate(ctx context.Context, req core.PolicyRequest) (core.PolicyDecision, error)
}

// ManifestPolicyEngine implements PolicyEngine using PermissionManager rules
// declared in an agent manifest.
type ManifestPolicyEngine struct {
	agentID string
	manager *PermissionManager
	rules   []core.PolicyRule
}

// FromManifestWithConfig constructs a ManifestPolicyEngine for the given agent.
// agentID identifies the agent in audit logs; manager carries the declared policy.
func FromManifestWithConfig(m *manifest.AgentManifest, agentID string, manager *PermissionManager) (*ManifestPolicyEngine, error) {
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

// Evaluate decides whether req should be allowed, denied, or routed to HITL.
//
// Trust class dispatch:
//   - BuiltinTrusted / WorkspaceTrusted → always allow (declared in manifest or built in).
//   - All remote / untrusted classes → apply the agent's configured default policy.
//     Allow → pass through; Deny → hard block; Ask (default) → require approval.
func (e *ManifestPolicyEngine) Evaluate(_ context.Context, req core.PolicyRequest) (core.PolicyDecision, error) {
	if e == nil {
		return core.PolicyDecisionAllow("no policy manager"), nil
	}
	if decision := evaluateCompiledRules(e.rules, req); decision != nil {
		e.emitDecision(context.Background(), req, *decision)
		return *decision, nil
	}
	if e.manager == nil {
		return core.PolicyDecisionAllow("no policy manager"), nil
	}
	decision := e.fallbackDecision(req)
	e.emitDecision(context.Background(), req, decision)
	return decision, nil
}

func (e *ManifestPolicyEngine) fallbackDecision(req core.PolicyRequest) core.PolicyDecision {
	switch req.Target {
	case core.PolicyTargetProvider:
		return e.providerFallbackDecision(req)
	default:
		return e.capabilityFallbackDecision(req)
	}
}

func (e *ManifestPolicyEngine) capabilityFallbackDecision(req core.PolicyRequest) core.PolicyDecision {
	switch req.TrustClass {
	case core.TrustClassBuiltinTrusted, core.TrustClassWorkspaceTrusted:
		return core.PolicyDecisionAllow("workspace trusted")
	default:
		switch e.manager.DefaultPolicy() {
		case core.AgentPermissionAllow:
			return core.PolicyDecisionAllow("default policy: allow")
		case core.AgentPermissionDeny:
			return core.PolicyDecisionDeny(
				fmt.Sprintf("capability %q denied by default policy for agent %s", req.CapabilityName, e.agentID),
			)
		default:
			return core.PolicyDecisionRequireApproval(nil)
		}
	}
}

func (e *ManifestPolicyEngine) providerFallbackDecision(req core.PolicyRequest) core.PolicyDecision {
	switch req.ProviderKind {
	case core.ProviderKindBuiltin, core.ProviderKindAgentRuntime:
		return core.PolicyDecisionAllow("provider kind trusted by default")
	}
	if req.ProviderOrigin == core.ProviderOriginRemote ||
		req.ProviderKind == core.ProviderKindMCPClient ||
		req.ProviderKind == core.ProviderKindMCPServer {
		return core.PolicyDecisionRequireApproval(nil)
	}
	return core.PolicyDecisionAllow("provider allowed by default")
}

func (e *ManifestPolicyEngine) emitDecision(ctx context.Context, req core.PolicyRequest, decision core.PolicyDecision) {
	if e == nil || e.manager == nil {
		return
	}
	desc := core.PermissionDescriptor{
		Type:     core.PermissionTypeCapability,
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

func permissionActionForRequest(req core.PolicyRequest) string {
	switch {
	case req.CapabilityName != "":
		return req.CapabilityName
	case req.CapabilityID != "":
		return req.CapabilityID
	case req.Target == core.PolicyTargetSession:
		return "session:" + string(req.SessionOperation)
	case req.Target == core.PolicyTargetProvider:
		return "provider"
	default:
		return "capability"
	}
}

func permissionResourceForRequest(req core.PolicyRequest) string {
	switch {
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
