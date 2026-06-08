package registry

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/governance/policy"
)

// PermissionManagerHandle and PolicyEngine are consumer-defined ports: the
// capability domain declares exactly what it calls on the authorization
// permission manager / policy engine, typed with the canonical governance/policy
// vocabulary so *authorization.PermissionManager and authorization.PolicyEngine
// satisfy them structurally without capability importing authorization.
type PermissionManagerHandle interface {
	RequireApproval(ctx context.Context, agentID string, desc permissions.PermissionDescriptor, justification string, scope policy.GrantScope, risk policy.RiskLevel, duration time.Duration) error
	AuthorizeTool(ctx context.Context, agentID string, tool any, args map[string]any) error
}

type PolicyEngine interface {
	Evaluate(ctx context.Context, req policy.PolicyRequest) (policy.PolicyDecision, error)
}

type ApprovalRequest struct {
	AgentID            string
	Manager            PermissionManagerHandle
	Permission         permissions.PermissionDescriptor
	Justification      string
	Scope              policy.GrantScope
	Risk               policy.RiskLevel
	Duration           time.Duration
	MissingManagerErr  string
	DenyReasonFallback string
}

func EnforcePolicyRequest(ctx context.Context, engine PolicyEngine, req policy.PolicyRequest, approval ApprovalRequest) (policy.PolicyDecision, error) {
	if engine == nil {
		return policy.PolicyDecision{Effect: "allow", Reason: "no policy engine"}, nil
	}
	decision, err := engine.Evaluate(ctx, req)
	if err != nil {
		return policy.PolicyDecision{}, err
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
