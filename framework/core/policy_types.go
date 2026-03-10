package core

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// PolicyRule is a declarative security rule evaluated at invocation time.
type PolicyRule struct {
	ID         string           `json:"id" yaml:"id"`
	Name       string           `json:"name" yaml:"name"`
	Priority   int              `json:"priority" yaml:"priority"`
	Enabled    bool             `json:"enabled" yaml:"enabled"`
	Conditions PolicyConditions `json:"conditions" yaml:"conditions"`
	Effect     PolicyEffect     `json:"effect" yaml:"effect"`
}

type PolicyConditions struct {
	Actors            []ActorMatch              `json:"actors,omitempty" yaml:"actors,omitempty"`
	Capabilities      []string                  `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	ProviderKinds     []ProviderKind            `json:"provider_kinds,omitempty" yaml:"provider_kinds,omitempty"`
	MinRiskClasses    []RiskClass               `json:"min_risk_classes,omitempty" yaml:"min_risk_classes,omitempty"`
	TrustClasses      []TrustClass              `json:"trust_classes,omitempty" yaml:"trust_classes,omitempty"`
	CapabilityKinds   []CapabilityKind          `json:"capability_kinds,omitempty" yaml:"capability_kinds,omitempty"`
	RuntimeFamilies   []CapabilityRuntimeFamily `json:"runtime_families,omitempty" yaml:"runtime_families,omitempty"`
	EffectClasses     []EffectClass             `json:"effect_classes,omitempty" yaml:"effect_classes,omitempty"`
	Partitions        []string                  `json:"partitions,omitempty" yaml:"partitions,omitempty"`
	ChannelIDs        []string                  `json:"channel_ids,omitempty" yaml:"channel_ids,omitempty"`
	SessionScopes     []SessionScope            `json:"session_scopes,omitempty" yaml:"session_scopes,omitempty"`
	SessionOperations []SessionOperation        `json:"session_operations,omitempty" yaml:"session_operations,omitempty"`
	RequireOwnership  *bool                     `json:"require_ownership,omitempty" yaml:"require_ownership,omitempty"`
	TimeWindow        *TimeWindow               `json:"time_window,omitempty" yaml:"time_window,omitempty"`
}

type ActorMatch struct {
	Kind          string   `json:"kind,omitempty" yaml:"kind,omitempty"`
	IDs           []string `json:"ids,omitempty" yaml:"ids,omitempty"`
	Authenticated bool     `json:"authenticated,omitempty" yaml:"authenticated,omitempty"`
}

type TimeWindow struct {
	After    string   `json:"after,omitempty" yaml:"after,omitempty"`
	Before   string   `json:"before,omitempty" yaml:"before,omitempty"`
	Days     []string `json:"days,omitempty" yaml:"days,omitempty"`
	Timezone string   `json:"timezone,omitempty" yaml:"timezone,omitempty"`
}

type PolicyEffect struct {
	Action      string     `json:"action" yaml:"action"`
	Approvers   []string   `json:"approvers,omitempty" yaml:"approvers,omitempty"`
	ApprovalTTL string     `json:"approval_ttl,omitempty" yaml:"approval_ttl,omitempty"`
	RateLimit   *RateLimit `json:"rate_limit,omitempty" yaml:"rate_limit,omitempty"`
	Reason      string     `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type RateLimit struct {
	MaxRequests   int    `json:"max_requests" yaml:"max_requests"`
	WindowSeconds int    `json:"window_seconds" yaml:"window_seconds"`
	Per           string `json:"per" yaml:"per"`
}

type PolicyRequest struct {
	Target           PolicyTarget
	Actor            EventActor
	Authenticated    bool
	ActorTenantID    string
	ResourceTenantID string
	CapabilityID     string
	CapabilityName   string
	CapabilityKind   CapabilityKind
	RuntimeFamily    CapabilityRuntimeFamily
	ProviderKind     ProviderKind
	ProviderOrigin   ProviderOriginKind
	TrustClass       TrustClass
	RiskClasses      []RiskClass
	EffectClasses    []EffectClass
	Partition        string
	ChannelID        string
	SessionID        string
	SessionScope     SessionScope
	SessionOperation SessionOperation
	SessionOwnerID   string
	IsOwner          bool
	Timestamp        time.Time
}

type PolicyTarget string

const (
	PolicyTargetCapability PolicyTarget = "capability"
	PolicyTargetProvider   PolicyTarget = "provider"
	PolicyTargetSession    PolicyTarget = "session"
)

type PolicyDecision struct {
	Effect string
	Rule   *PolicyRule
	Reason string
}

func PolicyDecisionAllow(reason string) PolicyDecision {
	return PolicyDecision{Effect: "allow", Reason: reason}
}

func PolicyDecisionDeny(reason string) PolicyDecision {
	return PolicyDecision{Effect: "deny", Reason: reason}
}

func PolicyDecisionRequireApproval(rule *PolicyRule) PolicyDecision {
	reason := ""
	if rule != nil {
		reason = rule.Effect.Reason
	}
	return PolicyDecision{Effect: "require_approval", Rule: rule, Reason: reason}
}

func (r PolicyRule) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return errors.New("policy rule id required")
	}
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("policy rule name required")
	}
	if err := r.Effect.Validate(); err != nil {
		return fmt.Errorf("policy effect invalid: %w", err)
	}
	if r.Conditions.TimeWindow != nil {
		if err := r.Conditions.TimeWindow.Validate(); err != nil {
			return fmt.Errorf("time window invalid: %w", err)
		}
	}
	for _, partition := range r.Conditions.Partitions {
		if strings.TrimSpace(partition) == "" {
			return errors.New("policy partition must not be empty")
		}
	}
	for _, channelID := range r.Conditions.ChannelIDs {
		if strings.TrimSpace(channelID) == "" {
			return errors.New("policy channel id must not be empty")
		}
	}
	return nil
}

func (e PolicyEffect) Validate() error {
	switch e.Action {
	case "allow", "deny", "require_approval", "rate_limit", "log_only":
	default:
		return fmt.Errorf("policy action %s invalid", e.Action)
	}
	if e.ApprovalTTL != "" {
		if _, err := time.ParseDuration(e.ApprovalTTL); err != nil {
			return fmt.Errorf("approval ttl invalid: %w", err)
		}
	}
	if e.Action == "rate_limit" {
		if e.RateLimit == nil {
			return errors.New("rate limit config required")
		}
		if err := e.RateLimit.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (r RateLimit) Validate() error {
	if r.MaxRequests <= 0 {
		return errors.New("max_requests must be > 0")
	}
	if r.WindowSeconds <= 0 {
		return errors.New("window_seconds must be > 0")
	}
	switch r.Per {
	case "actor", "capability", "global":
	default:
		return fmt.Errorf("rate limit per %s invalid", r.Per)
	}
	return nil
}

func (w TimeWindow) Validate() error {
	if strings.TrimSpace(w.After) == "" && strings.TrimSpace(w.Before) == "" {
		return errors.New("after or before required")
	}
	if w.After != "" {
		if _, err := time.Parse("15:04", w.After); err != nil {
			return fmt.Errorf("after invalid: %w", err)
		}
	}
	if w.Before != "" {
		if _, err := time.Parse("15:04", w.Before); err != nil {
			return fmt.Errorf("before invalid: %w", err)
		}
	}
	return nil
}
