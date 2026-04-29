package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	nexusadmin "codeburg.org/lexbit/relurpify/app/nexus/admin"
	nexuscfg "codeburg.org/lexbit/relurpify/app/nexus/config"
	fwgateway "codeburg.org/lexbit/relurpify/relurpnet/gateway"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

type gatewayPrincipalResolverImpl struct {
	staticBindings []identity.StaticTokenBinding
	tokenStore     nexusadmin.TokenStore
	identityStore  identity.Store
}

func gatewayPrincipalResolver(cfg nexuscfg.GatewayAuthConfig, tokenStore nexusadmin.TokenStore, identityStore identity.Store) func(context.Context, string) (fwgateway.ConnectionPrincipal, error) {
	if !cfg.Enabled {
		return nil
	}
	resolver := &gatewayPrincipalResolverImpl{
		staticBindings: make([]identity.StaticTokenBinding, 0, len(cfg.Tokens)),
		tokenStore:     tokenStore,
		identityStore:  identityStore,
	}
	for _, entry := range cfg.Tokens {
		if entry.Token == "" || entry.SubjectID == "" || entry.Role == "" {
			continue
		}
		resolver.staticBindings = append(resolver.staticBindings, identity.StaticTokenBinding{
			Token:       entry.Token,
			TenantID:    entry.TenantID,
			Role:        entry.Role,
			SubjectKind: identity.SubjectKind(entry.SubjectKind),
			SubjectID:   entry.SubjectID,
			Scopes:      append([]string(nil), entry.Scopes...),
		})
	}
	return resolver.Resolve
}

func (r *gatewayPrincipalResolverImpl) Resolve(ctx context.Context, token string) (fwgateway.ConnectionPrincipal, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return fwgateway.ConnectionPrincipal{}, fmt.Errorf("token required")
	}
	if principal, ok := r.resolveStatic(token); ok {
		return principal, nil
	}
	if r.tokenStore == nil {
		return fwgateway.ConnectionPrincipal{}, fmt.Errorf("token not recognized")
	}
	record, err := r.tokenStore.GetTokenByHash(ctx, hashToken(token))
	if err != nil {
		return fwgateway.ConnectionPrincipal{}, err
	}
	if record == nil {
		return fwgateway.ConnectionPrincipal{}, fmt.Errorf("token not recognized")
	}
	return r.resolveRecord(ctx, *record)
}

func (r *gatewayPrincipalResolverImpl) resolveStatic(token string) (fwgateway.ConnectionPrincipal, bool) {
	for _, binding := range r.staticBindings {
		if binding.Token != token {
			continue
		}
		principal := authPrincipalFromBinding(binding)
		return principal, true
	}
	return fwgateway.ConnectionPrincipal{}, false
}

func (r *gatewayPrincipalResolverImpl) resolveRecord(ctx context.Context, record identity.AdminTokenRecord) (fwgateway.ConnectionPrincipal, error) {
	tenantID := strings.TrimSpace(record.TenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	subjectKind := record.SubjectKind
	if subjectKind == "" {
		subjectKind = identity.SubjectKindServiceAccount
	}
	principal := identity.AuthenticatedPrincipal{
		TenantID:      tenantID,
		Authenticated: true,
		AuthMethod:    identity.AuthMethodBearerToken,
		Scopes:        append([]string(nil), record.Scopes...),
		Subject: identity.SubjectRef{
			TenantID: tenantID,
			Kind:     subjectKind,
			ID:       strings.TrimSpace(record.SubjectID),
		},
	}
	if r.identityStore != nil {
		subject, err := r.identityStore.GetSubject(ctx, tenantID, subjectKind, record.SubjectID)
		if err != nil {
			return fwgateway.ConnectionPrincipal{}, err
		}
		if subject != nil {
			principal.Subject = identity.SubjectRef{
				TenantID: subject.TenantID,
				Kind:     subject.Kind,
				ID:       subject.ID,
			}
		}
	}
	role := principalRoleFromRecord(record, principal.Scopes)
	return principalFromAuthenticated(role, principal), nil
}

func authPrincipalFromBinding(binding identity.StaticTokenBinding) fwgateway.ConnectionPrincipal {
	tenantID := strings.TrimSpace(binding.TenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	kind := binding.SubjectKind
	if kind == "" {
		kind = identity.SubjectKindServiceAccount
	}
	principal := identity.AuthenticatedPrincipal{
		TenantID:      tenantID,
		Authenticated: true,
		AuthMethod:    identity.AuthMethodBearerToken,
		Scopes:        append([]string(nil), binding.Scopes...),
		Subject: identity.SubjectRef{
			TenantID: tenantID,
			Kind:     kind,
			ID:       strings.TrimSpace(binding.SubjectID),
		},
	}
	return principalFromAuthenticated(binding.Role, principal)
}

func principalFromAuthenticated(role string, principal identity.AuthenticatedPrincipal) fwgateway.ConnectionPrincipal {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		role = "user"
	}
	actor := identity.EventActor{
		Kind:        string(principal.Subject.Kind),
		ID:          principal.Subject.ID,
		TenantID:    principal.TenantID,
		SessionID:   principal.SessionID,
		Scopes:      append([]string(nil), principal.Scopes...),
		SubjectKind: string(principal.Subject.Kind),
	}
	if actor.Kind == "" {
		actor.Kind = role
	}
	if actor.ID == "" {
		actor.ID = principal.Subject.ID
	}
	return fwgateway.ConnectionPrincipal{
		Role:          role,
		Actor:         actor,
		Authenticated: true,
		Principal:     &principal,
	}
}

func principalRoleFromRecord(record identity.AdminTokenRecord, scopes []string) string {
	for _, scope := range scopes {
		switch strings.ToLower(strings.TrimSpace(scope)) {
		case "admin", "operator", "gateway:admin", "nexus:admin":
			return "admin"
		}
	}
	if record.SubjectKind == identity.SubjectKindNode {
		return "node"
	}
	return "user"
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func stdioAdminPrincipal(cfg nexuscfg.Config, tokenStore nexusadmin.TokenStore, identityStore identity.Store, token string) (identity.AuthenticatedPrincipal, error) {
	resolver := gatewayPrincipalResolver(cfg.Gateway.Auth, tokenStore, identityStore)
	if strings.TrimSpace(token) != "" {
		if resolver == nil {
			return identity.AuthenticatedPrincipal{}, fmt.Errorf("gateway auth disabled")
		}
		principal, err := resolver(context.Background(), token)
		if err != nil || principal.Principal == nil {
			return identity.AuthenticatedPrincipal{}, fmt.Errorf("resolve admin principal: %w", err)
		}
		return *principal.Principal, nil
	}
	for _, entry := range cfg.Gateway.Auth.Tokens {
		role := strings.ToLower(strings.TrimSpace(entry.Role))
		if role != "admin" && role != "operator" {
			continue
		}
		if resolver == nil {
			break
		}
		principal, err := resolver(context.Background(), entry.Token)
		if err == nil && principal.Principal != nil {
			return *principal.Principal, nil
		}
	}
	return identity.AuthenticatedPrincipal{
		TenantID:      "default",
		AuthMethod:    identity.AuthMethodBootstrapAdmin,
		Authenticated: true,
		Scopes:        []string{"nexus:admin"},
		Subject: identity.SubjectRef{
			TenantID: "default",
			Kind:     identity.SubjectKindServiceAccount,
			ID:       "local-admin",
		},
	}, nil
}
