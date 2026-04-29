package identity

import (
	"context"
	"strings"
)

// InboundMessage is the minimal interface needed for identity resolution.
// This avoids importing the channel package and creating import cycles.
type InboundMessage interface {
	GetChannel() string
	GetAccount() string
	GetSender() InboundSender
	GetConversation() InboundConversation
}

// InboundSender provides sender identity information.
type InboundSender interface {
	GetResolvedID() string
	GetChannelID() string
}

// InboundConversation provides conversation context.
type InboundConversation interface {
	GetID() string
	GetThreadID() string
}

type Resolution struct {
	TenantID string
	Owner    SubjectRef
	Binding  *ExternalSessionBinding
	State    ResolutionState
}

type ResolutionState string

const (
	ResolutionStateUnknown  ResolutionState = ""
	ResolutionStateResolved ResolutionState = "resolved"
	ResolutionStateUnbound  ResolutionState = "unbound"
)

func (r Resolution) Resolved() bool {
	return r.State == ResolutionStateResolved
}

func (r Resolution) Unbound() bool {
	return r.State == ResolutionStateUnbound
}

type Resolver interface {
	ResolveInbound(ctx context.Context, inbound InboundMessage) (Resolution, error)
}

type StoreResolver struct {
	Store           Store
	DefaultTenantID string
}

func (r StoreResolver) ResolveInbound(ctx context.Context, inbound InboundMessage) (Resolution, error) {
	provider, ok := providerForChannel(inbound.GetChannel())
	if !ok {
		return Resolution{}, nil
	}
	accountID := strings.TrimSpace(inbound.GetAccount())
	externalID := strings.TrimSpace(inbound.GetSender().GetResolvedID())
	if externalID == "" {
		externalID = strings.TrimSpace(inbound.GetSender().GetChannelID())
	}
	if externalID == "" {
		return Resolution{
			TenantID: normalizeTenantID(r.DefaultTenantID),
			Binding:  bindingForInbound(provider, inbound),
			State:    ResolutionStateUnbound,
		}, nil
	}
	tenantID := normalizeTenantID(r.DefaultTenantID)
	if r.Store != nil {
		identity, err := r.Store.GetExternalIdentity(ctx, tenantID, provider, accountID, externalID)
		if err != nil {
			return Resolution{}, err
		}
		if identity != nil {
			return Resolution{
				TenantID: identity.TenantID,
				Owner:    identity.Subject,
				Binding:  bindingForInbound(identity.Provider, inbound),
				State:    ResolutionStateResolved,
			}, nil
		}
		identity, err = r.lookupExternalIdentityAcrossTenants(ctx, tenantID, provider, accountID, externalID)
		if err != nil {
			return Resolution{}, err
		}
		if identity != nil {
			return Resolution{
				TenantID: identity.TenantID,
				Owner:    identity.Subject,
				Binding:  bindingForInbound(identity.Provider, inbound),
				State:    ResolutionStateResolved,
			}, nil
		}
	}
	return Resolution{
		TenantID: tenantID,
		Binding:  bindingForInbound(provider, inbound),
		State:    ResolutionStateUnbound,
	}, nil
}

func providerForChannel(channelID string) (ExternalProvider, bool) {
	switch strings.ToLower(strings.TrimSpace(channelID)) {
	case "discord":
		return ExternalProviderDiscord, true
	case "telegram":
		return ExternalProviderTelegram, true
	case "webchat":
		return ExternalProviderWebchat, true
	default:
		return "", false
	}
}

func bindingForInbound(provider ExternalProvider, inbound InboundMessage) *ExternalSessionBinding {
	return &ExternalSessionBinding{
		Provider:       provider,
		AccountID:      strings.TrimSpace(inbound.GetAccount()),
		ChannelID:      strings.TrimSpace(inbound.GetChannel()),
		ConversationID: strings.TrimSpace(inbound.GetConversation().GetID()),
		ThreadID:       strings.TrimSpace(inbound.GetConversation().GetThreadID()),
		ExternalUserID: firstNonEmpty(strings.TrimSpace(inbound.GetSender().GetResolvedID()), strings.TrimSpace(inbound.GetSender().GetChannelID())),
	}
}

func normalizeTenantID(tenantID string) string {
	if strings.TrimSpace(tenantID) == "" {
		return "local"
	}
	return strings.TrimSpace(tenantID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (r StoreResolver) lookupExternalIdentityAcrossTenants(ctx context.Context, defaultTenantID string, provider ExternalProvider, accountID, externalID string) (*ExternalIdentity, error) {
	if r.Store == nil {
		return nil, nil
	}
	tenants, err := r.Store.ListTenants(ctx)
	if err != nil {
		return nil, err
	}
	defaultTenantID = normalizeTenantID(defaultTenantID)
	for _, tenant := range tenants {
		tenantID := normalizeTenantID(tenant.ID)
		if tenantID == defaultTenantID {
			continue
		}
		identity, err := r.Store.GetExternalIdentity(ctx, tenantID, provider, accountID, externalID)
		if err != nil {
			return nil, err
		}
		if identity != nil {
			return identity, nil
		}
	}
	return nil, nil
}
