package rexctx

import (
	"context"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	rexcontrolplane "codeburg.org/lexbit/relurpify/named/rex/controlplane"
)

type TrustedExecutionContext struct {
	TenantID          string
	WorkloadClass     rexcontrolplane.WorkloadClass
	SensitivityClass  core.SensitivityClass
	FederationTargets []string
	SessionID         string
}

type TenantPolicy struct {
	SensitivityClass  core.SensitivityClass
	FederationTargets []string
}

type TenantPolicyReader interface {
	GetTenantPolicy(context.Context, string) (*TenantPolicy, error)
}

type TrustedContextResolver interface {
	Resolve(context.Context, core.EventActor) (TrustedExecutionContext, error)
}

type DefaultTrustedContextResolver struct {
	PolicyReader TenantPolicyReader
}

type contextKey struct{}

func WithTrustedExecutionContext(ctx context.Context, trusted TrustedExecutionContext) context.Context {
	return context.WithValue(ctx, contextKey{}, trusted)
}

func TrustedExecutionContextFromContext(ctx context.Context) (TrustedExecutionContext, bool) {
	if ctx == nil {
		return TrustedExecutionContext{}, false
	}
	trusted, ok := ctx.Value(contextKey{}).(TrustedExecutionContext)
	return trusted, ok
}

func (r DefaultTrustedContextResolver) Resolve(ctx context.Context, actor core.EventActor) (TrustedExecutionContext, error) {
	trusted := TrustedExecutionContext{
		TenantID:          firstNonEmpty(actor.TenantID, "default"),
		SessionID:         firstNonEmpty(actor.SessionID, actor.ID),
		WorkloadClass:     deriveWorkloadClass(actor),
		SensitivityClass:  core.SensitivityClassMedium,
		FederationTargets: nil,
	}
	if r.PolicyReader != nil && strings.TrimSpace(trusted.TenantID) != "" {
		policy, err := r.PolicyReader.GetTenantPolicy(ctx, trusted.TenantID)
		if err != nil {
			return TrustedExecutionContext{}, err
		}
		if policy != nil {
			if strings.TrimSpace(string(policy.SensitivityClass)) != "" {
				trusted.SensitivityClass = policy.SensitivityClass
			}
			if len(policy.FederationTargets) > 0 {
				trusted.FederationTargets = append([]string(nil), policy.FederationTargets...)
			}
		}
	}
	return trusted, nil
}

func deriveWorkloadClass(actor core.EventActor) rexcontrolplane.WorkloadClass {
	if isPrivilegedActor(actor) {
		return rexcontrolplane.WorkloadCritical
	}
	return rexcontrolplane.WorkloadBestEffort
}

func isPrivilegedActor(actor core.EventActor) bool {
	switch strings.ToLower(strings.TrimSpace(actor.Kind)) {
	case "operator", "admin", "nexus:operator", "nexus:admin", "gateway:admin":
		return true
	}
	for _, scope := range actor.Scopes {
		switch strings.ToLower(strings.TrimSpace(scope)) {
		case "rex:workload:critical", "operator", "admin", "nexus:operator", "nexus:admin", "gateway:admin":
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
