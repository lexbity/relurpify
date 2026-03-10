package identity

import (
	"context"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/middleware/channel"
)

type Resolution struct {
	TenantID string
	Owner    core.SubjectRef
	Binding  *core.ExternalSessionBinding
	Resolved bool
}

type Resolver interface {
	ResolveInbound(ctx context.Context, inbound channel.InboundMessage) (Resolution, error)
}

type StoreResolver struct {
	Store           Store
	DefaultTenantID string
}

func (r StoreResolver) ResolveInbound(ctx context.Context, inbound channel.InboundMessage) (Resolution, error) {
	provider, ok := providerForChannel(inbound.Channel)
	if !ok {
		return Resolution{}, nil
	}
	accountID := strings.TrimSpace(inbound.Account)
	externalID := strings.TrimSpace(inbound.Sender.ResolvedID)
	if externalID == "" {
		externalID = strings.TrimSpace(inbound.Sender.ChannelID)
	}
	if externalID == "" {
		return Resolution{
			TenantID: normalizeTenantID(r.DefaultTenantID),
			Binding:  bindingForInbound(provider, inbound),
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
				Resolved: true,
			}, nil
		}
	}
	return Resolution{
		TenantID: tenantID,
		Binding:  bindingForInbound(provider, inbound),
	}, nil
}

func providerForChannel(channelID string) (core.ExternalProvider, bool) {
	switch strings.ToLower(strings.TrimSpace(channelID)) {
	case "discord":
		return core.ExternalProviderDiscord, true
	case "telegram":
		return core.ExternalProviderTelegram, true
	case "webchat":
		return core.ExternalProviderWebchat, true
	default:
		return "", false
	}
}

func bindingForInbound(provider core.ExternalProvider, inbound channel.InboundMessage) *core.ExternalSessionBinding {
	return &core.ExternalSessionBinding{
		Provider:       provider,
		AccountID:      strings.TrimSpace(inbound.Account),
		ChannelID:      strings.TrimSpace(inbound.Channel),
		ConversationID: strings.TrimSpace(inbound.Conversation.ID),
		ThreadID:       strings.TrimSpace(inbound.Conversation.ThreadID),
		ExternalUserID: firstNonEmpty(strings.TrimSpace(inbound.Sender.ResolvedID), strings.TrimSpace(inbound.Sender.ChannelID)),
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
