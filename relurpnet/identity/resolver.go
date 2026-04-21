package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	fwgateway "codeburg.org/lexbit/relurpify/relurpnet/gateway"
)

type Resolver struct {
	staticByHash map[string]StaticTokenBinding
	tokenStore   TokenLookupStore
	subjectStore SubjectLookupStore
	now          func() time.Time
}

func NewResolver(static []StaticTokenBinding, tokenStore TokenLookupStore, subjectStore SubjectLookupStore) *Resolver {
	staticByHash := make(map[string]StaticTokenBinding, len(static))
	for _, binding := range static {
		if strings.TrimSpace(binding.Token) == "" || strings.TrimSpace(binding.Role) == "" || strings.TrimSpace(binding.SubjectID) == "" {
			continue
		}
		staticByHash[hashToken(binding.Token)] = binding
	}
	return &Resolver{
		staticByHash: staticByHash,
		tokenStore:   tokenStore,
		subjectStore: subjectStore,
		now:          time.Now,
	}
}

func (r *Resolver) ResolvePrincipal(ctx context.Context, token string) (fwgateway.ConnectionPrincipal, error) {
	if strings.TrimSpace(token) == "" {
		return fwgateway.ConnectionPrincipal{}, NewResolutionError(ResolutionErrorBearerTokenRequired, "bearer token required")
	}
	hashed := hashToken(token)
	if binding, ok := r.staticByHash[hashed]; ok {
		return r.resolveStaticBinding(binding)
	}
	if r.tokenStore == nil {
		return fwgateway.ConnectionPrincipal{}, NewResolutionError(ResolutionErrorUnknownToken, "unknown bearer token").WithDetail("token_hash", hashed)
	}
	record, err := r.tokenStore.GetTokenByHash(ctx, hashed)
	if err != nil {
		return fwgateway.ConnectionPrincipal{}, WrapResolutionError(ResolutionErrorTokenLookupFailed, "lookup bearer token failed", err).WithDetail("token_hash", hashed)
	}
	if record == nil {
		return fwgateway.ConnectionPrincipal{}, NewResolutionError(ResolutionErrorUnknownToken, "unknown bearer token").WithDetail("token_hash", hashed)
	}
	now := r.now
	if now == nil {
		now = time.Now
	}
	if record.RevokedAt != nil {
		return fwgateway.ConnectionPrincipal{}, NewResolutionError(ResolutionErrorTokenRevoked, "bearer token revoked").WithDetail("token_id", record.ID)
	}
	if record.ExpiresAt != nil && record.ExpiresAt.Before(now().UTC()) {
		return fwgateway.ConnectionPrincipal{}, NewResolutionError(ResolutionErrorTokenExpired, "bearer token expired").WithDetail("token_id", record.ID)
	}
	principal, err := r.resolveRecordPrincipal(ctx, *record)
	if err != nil {
		return fwgateway.ConnectionPrincipal{}, err
	}
	return principal, nil
}

func (r *Resolver) resolveStaticBinding(binding StaticTokenBinding) (fwgateway.ConnectionPrincipal, error) {
	subjectKind := binding.SubjectKind
	if subjectKind == "" {
		return fwgateway.ConnectionPrincipal{}, NewResolutionError(ResolutionErrorAmbiguousSubject, "static token subject kind required").WithDetail("role", strings.TrimSpace(binding.Role)).WithDetail("subject_id", strings.TrimSpace(binding.SubjectID))
	}
	tenantID := normalizeTenantID(binding.TenantID)
	principal := core.AuthenticatedPrincipal{
		TenantID:      tenantID,
		AuthMethod:    core.AuthMethodBearerToken,
		Authenticated: true,
		Scopes:        append([]string(nil), binding.Scopes...),
		Subject: core.SubjectRef{
			TenantID: tenantID,
			Kind:     subjectKind,
			ID:       strings.TrimSpace(binding.SubjectID),
		},
	}
	if err := principal.Validate(); err != nil {
		return fwgateway.ConnectionPrincipal{}, WrapResolutionError(ResolutionErrorInvalidPrincipal, "static token principal invalid", err).WithDetail("tenant_id", tenantID).WithDetail("subject_id", strings.TrimSpace(binding.SubjectID))
	}
	return fwgateway.ConnectionPrincipal{
		Role:          strings.TrimSpace(binding.Role),
		Authenticated: true,
		Principal:     &principal,
		Actor: core.EventActor{
			Kind:        strings.TrimSpace(binding.Role),
			ID:          strings.TrimSpace(binding.SubjectID),
			TenantID:    tenantID,
			SubjectKind: subjectKind,
		},
	}, nil
}

func (r *Resolver) resolveRecordPrincipal(ctx context.Context, record core.AdminTokenRecord) (fwgateway.ConnectionPrincipal, error) {
	tenantID := normalizeTenantID(record.TenantID)
	subjectID := strings.TrimSpace(record.SubjectID)
	if subjectID == "" {
		return fwgateway.ConnectionPrincipal{}, NewResolutionError(ResolutionErrorInvalidPrincipal, "token missing subject id").WithDetail("token_id", record.ID)
	}
	subjectKind := record.SubjectKind
	if subjectKind == "" {
		return fwgateway.ConnectionPrincipal{}, NewResolutionError(ResolutionErrorAmbiguousSubject, "token subject kind required").WithDetail("token_id", record.ID).WithDetail("subject_id", subjectID)
	}
	if r.subjectStore != nil {
		tenant, err := r.subjectStore.GetTenant(ctx, tenantID)
		if err != nil {
			return fwgateway.ConnectionPrincipal{}, WrapResolutionError(ResolutionErrorTenantLookupFailed, "lookup token tenant failed", err).WithDetail("tenant_id", tenantID)
		}
		if tenant == nil {
			return fwgateway.ConnectionPrincipal{}, NewResolutionError(ResolutionErrorDisabledTenant, fmt.Sprintf("token tenant %s not found", tenantID)).WithDetail("tenant_id", tenantID)
		}
		if tenant.DisabledAt != nil {
			return fwgateway.ConnectionPrincipal{}, NewResolutionError(ResolutionErrorDisabledTenant, fmt.Sprintf("token tenant %s disabled", tenantID)).WithDetail("tenant_id", tenantID)
		}
		subject, err := r.subjectStore.GetSubject(ctx, tenantID, subjectKind, subjectID)
		if err != nil {
			return fwgateway.ConnectionPrincipal{}, WrapResolutionError(ResolutionErrorSubjectLookupFailed, "lookup token subject failed", err).WithDetail("tenant_id", tenantID).WithDetail("subject_kind", subjectKind).WithDetail("subject_id", subjectID)
		}
		if subject == nil {
			return fwgateway.ConnectionPrincipal{}, NewResolutionError(ResolutionErrorUnknownToken, fmt.Sprintf("token subject %s/%s not found", subjectKind, subjectID)).WithDetail("tenant_id", tenantID).WithDetail("subject_kind", subjectKind).WithDetail("subject_id", subjectID)
		}
		if subject.DisabledAt != nil {
			return fwgateway.ConnectionPrincipal{}, NewResolutionError(ResolutionErrorDisabledSubject, fmt.Sprintf("token subject %s/%s disabled", subjectKind, subjectID)).WithDetail("tenant_id", tenantID).WithDetail("subject_kind", subjectKind).WithDetail("subject_id", subjectID)
		}
	}
	principal := core.AuthenticatedPrincipal{
		TenantID:      tenantID,
		AuthMethod:    core.AuthMethodBearerToken,
		Authenticated: true,
		Scopes:        append([]string(nil), record.Scopes...),
		Subject: core.SubjectRef{
			TenantID: tenantID,
			Kind:     subjectKind,
			ID:       subjectID,
		},
	}
	if err := principal.Validate(); err != nil {
		return fwgateway.ConnectionPrincipal{}, WrapResolutionError(ResolutionErrorInvalidPrincipal, "token principal invalid", err).WithDetail("token_id", record.ID)
	}
	role := principalRole(record.Scopes)
	return fwgateway.ConnectionPrincipal{
		Role:          role,
		Authenticated: true,
		Principal:     &principal,
		Actor: core.EventActor{
			Kind:        role,
			ID:          principal.Subject.ID,
			TenantID:    principal.TenantID,
			SubjectKind: principal.Subject.Kind,
		},
	}, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func normalizeTenantID(tenantID string) string {
	if strings.TrimSpace(tenantID) == "" {
		return "local"
	}
	return strings.TrimSpace(tenantID)
}

func principalRole(scopes []string) string {
	role := "agent"
	for _, scope := range scopes {
		switch strings.ToLower(strings.TrimSpace(scope)) {
		case "gateway:admin", "nexus:admin", "admin":
			return "admin"
		case "nexus:operator", "operator":
			role = "operator"
		case "node":
			if role == "agent" {
				role = "node"
			}
		}
	}
	return role
}
