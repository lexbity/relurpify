package policy

import (
	"errors"
	"fmt"
	"strings"
	"time"

	agentspec "codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/governance/identity"
)

// PolicyRule is a declarative security rule evaluated at invocation time.
type PolicyRule struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Priority   int              `json:"priority"`
	Enabled    bool             `json:"enabled"`
	Conditions PolicyConditions `json:"conditions"`
	Effect     PolicyEffect     `json:"effect"`
}

type PolicyConditions struct {
	Actors                    []ActorMatch                        `yaml:"actors,omitempty"`
	Capabilities              []string                            `yaml:"capabilities,omitempty"`
	ExportNames               []string                            `yaml:"export_names,omitempty"`
	SourceDomains             []string                            `yaml:"source_domains,omitempty"`
	ContextClasses            []string                            `yaml:"context_classes,omitempty"`
	SensitivityClasses        []string                            `yaml:"sensitivity_classes,omitempty"`
	RouteModes                []string                            `yaml:"route_modes,omitempty"`
	ProviderKinds             []agentspec.ProviderKind            `yaml:"provider_kinds,omitempty"`
	ExternalProviders         []string                            `yaml:"external_providers,omitempty"`
	MinRiskClasses            []agentspec.RiskClass               `yaml:"min_risk_classes,omitempty"`
	TrustClasses              []agentspec.TrustClass              `yaml:"trust_classes,omitempty"`
	CapabilityKinds           []agentspec.CapabilityKind          `yaml:"capability_kinds,omitempty"`
	RuntimeFamilies           []agentspec.CapabilityRuntimeFamily `yaml:"runtime_families,omitempty"`
	EffectClasses             []agentspec.EffectClass             `yaml:"effect_classes,omitempty"`
	Partitions                []string                            `yaml:"partitions,omitempty"`
	ChannelIDs                []string                            `yaml:"channel_ids,omitempty"`
	SessionScopes             []SessionScope                      `yaml:"session_scopes,omitempty"`
	SessionOperations         []SessionOperation                  `yaml:"session_operations,omitempty"`
	RequireOwnership          *bool                               `yaml:"require_ownership,omitempty"`
	RequireDelegation         *bool                               `yaml:"require_delegation,omitempty"`
	RequireExternalBinding    *bool                               `yaml:"require_external_binding,omitempty"`
	RequireResolvedExternal   *bool                               `yaml:"require_resolved_external,omitempty"`
	RequireRestrictedExternal *bool                               `yaml:"require_restricted_external,omitempty"`
	TimeWindow                *TimeWindow                         `yaml:"time_window,omitempty"`
}

type ActorMatch struct {
	Kind          string   `json:"kind,omitempty"`
	IDs           []string `json:"ids,omitempty"`
	Authenticated bool     `json:"authenticated,omitempty"`
}

type TimeWindow struct {
	After    string   `json:"after,omitempty"`
	Before   string   `json:"before,omitempty"`
	Days     []string `json:"days,omitempty"`
	Timezone string   `json:"timezone,omitempty"`
}

type PolicyEffect struct {
	Action      string     `yaml:"action"`
	Approvers   []string   `yaml:"approvers,omitempty"`
	ApprovalTTL string     `yaml:"approval_ttl,omitempty"`
	RateLimit   *RateLimit `yaml:"rate_limit,omitempty"`
	Reason      string     `yaml:"reason,omitempty"`
}

type RateLimit struct {
	MaxRequests   int    `yaml:"max_requests"`
	WindowSeconds int    `yaml:"window_seconds"`
	Per           string `yaml:"per"`
}

type PolicyRequest struct {
	Target                 PolicyTarget
	Actor                  identity.EventActor
	Authenticated          bool
	ActorTenantID          string
	ResourceTenantID       string
	CapabilityID           string
	CapabilityName         string
	LineageID              string
	AttemptID              string
	ExportName             string
	SourceDomain           string
	ContextClass           string
	SensitivityClass       string
	RouteMode              string
	CapabilityKind         agentspec.CapabilityKind
	RuntimeFamily          agentspec.CapabilityRuntimeFamily
	ProviderKind           agentspec.ProviderKind
	ProviderOrigin         agentspec.ProviderOriginKind
	TrustClass             agentspec.TrustClass
	RiskClasses            []agentspec.RiskClass
	EffectClasses          []agentspec.EffectClass
	Partition              string
	ChannelID              string
	SessionID              string
	SessionScope           SessionScope
	SessionOperation       SessionOperation
	SessionOwnerID         string
	IsOwner                bool
	IsDelegated            bool
	ExternalProvider       string
	ExternalAccountID      string
	ExternalChannelID      string
	ExternalConversationID string
	ExternalThreadID       string
	ExternalUserID         string
	HasExternalBinding     bool
	ResolvedExternal       bool
	RestrictedExternal     bool
	Timestamp              time.Time
}

type PolicyTarget string

const (
	PolicyTargetCapability PolicyTarget = "capability"
	PolicyTargetProvider   PolicyTarget = "provider"
	PolicyTargetSession    PolicyTarget = "session"
	PolicyTargetResume     PolicyTarget = "resume"
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
	for _, exportName := range r.Conditions.ExportNames {
		if strings.TrimSpace(exportName) == "" {
			return errors.New("policy export name must not be empty")
		}
	}
	for _, contextClass := range r.Conditions.ContextClasses {
		if strings.TrimSpace(contextClass) == "" {
			return errors.New("policy context class must not be empty")
		}
	}
	for _, sensitivityClass := range r.Conditions.SensitivityClasses {
		if strings.TrimSpace(sensitivityClass) == "" {
			return errors.New("policy sensitivity class must not be empty")
		}
	}
	for _, routeMode := range r.Conditions.RouteModes {
		if strings.TrimSpace(routeMode) == "" {
			return errors.New("policy route mode must not be empty")
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
