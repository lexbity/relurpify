package core

import (
	"fmt"
	"strings"

	agentspec "codeburg.org/lexbit/relurpify/framework/agentspec"
)

// SessionOperation identifies the action being authorized against a session.
type SessionOperation string

const (
	SessionOperationAttach  SessionOperation = "attach"
	SessionOperationSend    SessionOperation = "send"
	SessionOperationInvoke  SessionOperation = "invoke"
	SessionOperationResume  SessionOperation = "resume"
	SessionOperationInspect SessionOperation = "inspect"
	SessionOperationClose   SessionOperation = "close"
)

// SessionSelector matches session-oriented requests during policy evaluation.
type SessionSelector struct {
	Partitions                []string           `yaml:"partitions,omitempty" json:"partitions,omitempty"`
	ChannelIDs                []string           `yaml:"channel_ids,omitempty" json:"channel_ids,omitempty"`
	Scopes                    []SessionScope     `yaml:"scopes,omitempty" json:"scopes,omitempty"`
	TrustClasses              []TrustClass       `yaml:"trust_classes,omitempty" json:"trust_classes,omitempty"`
	Operations                []SessionOperation `yaml:"operations,omitempty" json:"operations,omitempty"`
	ActorKinds                []string           `yaml:"actor_kinds,omitempty" json:"actor_kinds,omitempty"`
	ActorIDs                  []string           `yaml:"actor_ids,omitempty" json:"actor_ids,omitempty"`
	ExternalProviders         []string           `yaml:"external_providers,omitempty" json:"external_providers,omitempty"`
	RequireOwnership          *bool              `yaml:"require_ownership,omitempty" json:"require_ownership,omitempty"`
	RequireDelegation         *bool              `yaml:"require_delegation,omitempty" json:"require_delegation,omitempty"`
	RequireExternalBinding    *bool              `yaml:"require_external_binding,omitempty" json:"require_external_binding,omitempty"`
	RequireResolvedExternal   *bool              `yaml:"require_resolved_external,omitempty" json:"require_resolved_external,omitempty"`
	RequireRestrictedExternal *bool              `yaml:"require_restricted_external,omitempty" json:"require_restricted_external,omitempty"`
	AuthenticatedOnly         *bool              `yaml:"authenticated_only,omitempty" json:"authenticated_only,omitempty"`
}

// SessionPolicy configures access to session-scoped operations.
type SessionPolicy struct {
	ID          string                         `yaml:"id" json:"id"`
	Name        string                         `yaml:"name" json:"name"`
	Priority    int                            `yaml:"priority,omitempty" json:"priority,omitempty"`
	Enabled     bool                           `yaml:"enabled" json:"enabled"`
	Selector    SessionSelector                `yaml:"selector" json:"selector"`
	Effect      agentspec.AgentPermissionLevel `yaml:"effect" json:"effect"`
	Approvers   []string                       `yaml:"approvers,omitempty" json:"approvers,omitempty"`
	ApprovalTTL string                         `yaml:"approval_ttl,omitempty" json:"approval_ttl,omitempty"`
	Reason      string                         `yaml:"reason,omitempty" json:"reason,omitempty"`
}

// ValidateSessionPolicy validates a session policy definition.
func ValidateSessionPolicy(policy SessionPolicy) error {
	if strings.TrimSpace(policy.ID) == "" {
		return fmt.Errorf("id required")
	}
	if strings.TrimSpace(policy.Name) == "" {
		return fmt.Errorf("name required")
	}
	if err := ValidateSessionSelector(policy.Selector); err != nil {
		return fmt.Errorf("selector invalid: %w", err)
	}
	switch policy.Effect {
	case agentspec.AgentPermissionAllow, agentspec.AgentPermissionAsk, agentspec.AgentPermissionDeny:
	default:
		return fmt.Errorf("effect=%s invalid", policy.Effect)
	}
	for _, approver := range policy.Approvers {
		if strings.TrimSpace(approver) == "" {
			return fmt.Errorf("approvers contains empty approver")
		}
	}
	if policy.ApprovalTTL != "" {
		if _, err := parsePolicyDuration(policy.ApprovalTTL); err != nil {
			return fmt.Errorf("approval_ttl invalid: %w", err)
		}
	}
	return nil
}

// ValidateSessionSelector validates a session selector definition.
func ValidateSessionSelector(selector SessionSelector) error {
	if len(selector.Partitions) == 0 &&
		len(selector.ChannelIDs) == 0 &&
		len(selector.Scopes) == 0 &&
		len(selector.TrustClasses) == 0 &&
		len(selector.Operations) == 0 &&
		len(selector.ActorKinds) == 0 &&
		len(selector.ActorIDs) == 0 &&
		len(selector.ExternalProviders) == 0 &&
		selector.RequireOwnership == nil &&
		selector.RequireDelegation == nil &&
		selector.RequireExternalBinding == nil &&
		selector.RequireResolvedExternal == nil &&
		selector.RequireRestrictedExternal == nil &&
		selector.AuthenticatedOnly == nil {
		return fmt.Errorf("at least one selector field required")
	}
	for _, partition := range selector.Partitions {
		if strings.TrimSpace(partition) == "" {
			return fmt.Errorf("partitions contains empty partition")
		}
	}
	for _, channelID := range selector.ChannelIDs {
		if strings.TrimSpace(channelID) == "" {
			return fmt.Errorf("channel_ids contains empty channel id")
		}
	}
	for _, scope := range selector.Scopes {
		switch scope {
		case SessionScopeMain, SessionScopePerChannelPeer, SessionScopePerThread:
		default:
			return fmt.Errorf("scope %s invalid", scope)
		}
	}
	for _, trustClass := range selector.TrustClasses {
		if strings.TrimSpace(string(trustClass)) == "" {
			return fmt.Errorf("trust_classes contains empty trust class")
		}
	}
	for _, operation := range selector.Operations {
		switch operation {
		case SessionOperationAttach, SessionOperationSend, SessionOperationInvoke, SessionOperationResume, SessionOperationInspect, SessionOperationClose:
		default:
			return fmt.Errorf("operation %s invalid", operation)
		}
	}
	for _, actorKind := range selector.ActorKinds {
		if strings.TrimSpace(actorKind) == "" {
			return fmt.Errorf("actor_kinds contains empty actor kind")
		}
	}
	for _, actorID := range selector.ActorIDs {
		if strings.TrimSpace(actorID) == "" {
			return fmt.Errorf("actor_ids contains empty actor id")
		}
	}
	for _, provider := range selector.ExternalProviders {
		if strings.TrimSpace(provider) == "" {
			return fmt.Errorf("session selector external provider must not be empty")
		}
	}
	return nil
}
