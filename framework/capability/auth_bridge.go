package capability

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// PermissionManagerHandle and PolicyEngine are consumer-defined ports: the
// capability domain declares exactly what it calls on the authorization
// permission manager / policy engine, typed with the canonical governance/policy
// vocabulary so *authorization.PermissionManager and authorization.PolicyEngine
// satisfy them structurally without capability importing authorization.
type PermissionManagerHandle interface {
	RequireApproval(ctx context.Context, agentID string, desc contracts.PermissionDescriptor, justification string, scope GrantScope, risk RiskLevel, duration time.Duration) error
	AuthorizeTool(ctx context.Context, agentID string, tool contracts.Tool, args map[string]any) error
}

type PolicyEngine interface {
	Evaluate(ctx context.Context, req PolicyRequest) (PolicyDecision, error)
}

// Canonical governance/policy types, re-exported so capability call sites keep
// their short names.
type PolicyDecision = policy.PolicyDecision
type GrantScope = policy.GrantScope
type RiskLevel = policy.RiskLevel

const (
	GrantScopeOneTime = policy.GrantScopeOneTime
	GrantScopeSession = policy.GrantScopeSession
	RiskLevelMedium   = policy.RiskLevelMedium
)

type ApprovalRequest struct {
	AgentID            string
	Manager            PermissionManagerHandle
	Permission         contracts.PermissionDescriptor
	Justification      string
	Scope              GrantScope
	Risk               RiskLevel
	Duration           time.Duration
	MissingManagerErr  string
	DenyReasonFallback string
}

func EnforcePolicyRequest(ctx context.Context, engine PolicyEngine, req PolicyRequest, approval ApprovalRequest) (PolicyDecision, error) {
	if engine == nil {
		return PolicyDecision{Effect: "allow", Reason: "no policy engine"}, nil
	}
	decision, err := engine.Evaluate(ctx, req)
	if err != nil {
		return PolicyDecision{}, err
	}
	switch decision.Effect {
	case "", "allow":
		return decision, nil
	case "deny":
		reason := decision.Reason
		if reason == "" {
			reason = approval.DenyReasonFallback
		}
		if reason == "" {
			reason = "denied by policy"
		}
		return decision, fmt.Errorf("%s", reason)
	case "require_approval":
		if approval.Manager == nil {
			reason := approval.MissingManagerErr
			if reason == "" {
				reason = "approval required but permission manager unavailable"
			}
			return decision, fmt.Errorf("%s", reason)
		}
		if err := approval.Manager.RequireApproval(
			ctx,
			approval.AgentID,
			approval.Permission,
			approval.Justification,
			approval.Scope,
			approval.Risk,
			approval.Duration,
		); err != nil {
			return decision, err
		}
		return decision, nil
	default:
		return decision, fmt.Errorf("unsupported policy effect %q", decision.Effect)
	}
}
