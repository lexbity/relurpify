package main

import (
	"context"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/identity"
)

func upsertTenantAndSubject(ctx context.Context, store identity.Store, tenantID string, kind core.SubjectKind, subjectID, displayName string, roles []string, createdAt time.Time) error {
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
	if err := store.UpsertTenant(ctx, core.TenantRecord{
		ID:          tenantID,
		DisplayName: tenantID,
		CreatedAt:   createdAt,
	}); err != nil {
		return err
	}
	return store.UpsertSubject(ctx, core.SubjectRecord{
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
