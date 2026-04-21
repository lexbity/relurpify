package main

import (
	"context"
	"fmt"
	"strings"

	nexusadmin "codeburg.org/lexbit/relurpify/app/nexus/admin"
	nexuscfg "codeburg.org/lexbit/relurpify/app/nexus/config"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/identity"
	fwgateway "codeburg.org/lexbit/relurpify/relurpnet/gateway"
	mwidentity "codeburg.org/lexbit/relurpify/relurpnet/identity"
)

func gatewayPrincipalResolver(cfg nexuscfg.GatewayAuthConfig, tokenStore nexusadmin.TokenStore, identityStore identity.Store) func(context.Context, string) (fwgateway.ConnectionPrincipal, error) {
	if !cfg.Enabled {
		return nil
	}
	staticBindings := make([]mwidentity.StaticTokenBinding, 0, len(cfg.Tokens))
	for _, entry := range cfg.Tokens {
		if entry.Token == "" || entry.SubjectID == "" || entry.Role == "" {
			continue
		}
		staticBindings = append(staticBindings, mwidentity.StaticTokenBinding{
			Token:       entry.Token,
			TenantID:    entry.TenantID,
			Role:        entry.Role,
			SubjectKind: core.SubjectKind(entry.SubjectKind),
			SubjectID:   entry.SubjectID,
			Scopes:      append([]string(nil), entry.Scopes...),
		})
	}
	return mwidentity.NewResolver(staticBindings, tokenStore, identityStore).ResolvePrincipal
}

func stdioAdminPrincipal(cfg nexuscfg.Config, tokenStore nexusadmin.TokenStore, identityStore identity.Store, token string) (core.AuthenticatedPrincipal, error) {
	if strings.TrimSpace(token) != "" {
		resolver := gatewayPrincipalResolver(cfg.Gateway.Auth, tokenStore, identityStore)
		if resolver == nil {
			return core.AuthenticatedPrincipal{}, fmt.Errorf("gateway auth disabled")
		}
		principal, err := resolver(context.Background(), token)
		if err != nil || principal.Principal == nil {
			return core.AuthenticatedPrincipal{}, fmt.Errorf("resolve admin principal: %w", err)
		}
		return *principal.Principal, nil
	}
	for _, entry := range cfg.Gateway.Auth.Tokens {
		role := strings.ToLower(strings.TrimSpace(entry.Role))
		if role != "admin" && role != "operator" {
			continue
		}
		principal, err := stdioAdminPrincipal(cfg, tokenStore, identityStore, entry.Token)
		if err == nil {
			return principal, nil
		}
	}
	return core.AuthenticatedPrincipal{
		TenantID:      "default",
		AuthMethod:    core.AuthMethodBootstrapAdmin,
		Authenticated: true,
		Scopes:        []string{"nexus:admin"},
		Subject: core.SubjectRef{
			TenantID: "default",
			Kind:     core.SubjectKindServiceAccount,
			ID:       "local-admin",
		},
	}, nil
}
