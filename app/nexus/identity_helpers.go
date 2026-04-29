package main

import (
	"context"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

func upsertTenantAndSubject(ctx context.Context, store identity.Store, tenantID string, kind identity.SubjectKind, subjectID, displayName string, roles []string, createdAt time.Time) error {
	if store == nil {
		return nil
	}
	tenantID = strings.TrimSpace(tenantID)
	subjectID = strings.TrimSpace(subjectID)
	if tenantID == "" || subjectID == "" || kind == "" {
		return nil
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	if err := store.UpsertTenant(ctx, identity.TenantRecord{
		ID:          tenantID,
		DisplayName: tenantID,
		CreatedAt:   createdAt,
	}); err != nil {
		return err
	}
	return store.UpsertSubject(ctx, identity.SubjectRecord{
		TenantID:    tenantID,
		Kind:        kind,
		ID:          subjectID,
		DisplayName: firstNonEmpty(strings.TrimSpace(displayName), subjectID),
		Roles:       append([]string(nil), roles...),
		CreatedAt:   createdAt,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
