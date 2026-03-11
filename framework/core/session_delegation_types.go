package core

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// SessionDelegationRecord grants a tenant-scoped subject permission to act on a
// session they do not own for specific session operations.
type SessionDelegationRecord struct {
	TenantID   string             `json:"tenant_id" yaml:"tenant_id"`
	SessionID  string             `json:"session_id" yaml:"session_id"`
	Grantee    SubjectRef         `json:"grantee" yaml:"grantee"`
	Operations []SessionOperation `json:"operations,omitempty" yaml:"operations,omitempty"`
	CreatedAt  time.Time          `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	ExpiresAt  time.Time          `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

func (r SessionDelegationRecord) Validate() error {
	if strings.TrimSpace(r.TenantID) == "" {
		return errors.New("session delegation tenant_id required")
	}
	if strings.TrimSpace(r.SessionID) == "" {
		return errors.New("session delegation session_id required")
	}
	if err := r.Grantee.Validate(); err != nil {
		return fmt.Errorf("session delegation grantee invalid: %w", err)
	}
	if !strings.EqualFold(r.TenantID, r.Grantee.TenantID) {
		return errors.New("session delegation tenant_id must match grantee tenant_id")
	}
	for _, op := range r.Operations {
		if strings.TrimSpace(string(op)) == "" {
			return errors.New("session delegation operations must not contain empty values")
		}
	}
	if !r.ExpiresAt.IsZero() && !r.CreatedAt.IsZero() && r.ExpiresAt.Before(r.CreatedAt) {
		return errors.New("session delegation expires_at must be after created_at")
	}
	return nil
}

func (r SessionDelegationRecord) Allows(actor EventActor, operation SessionOperation, now time.Time) bool {
	if !r.Grantee.Matches(actor) {
		return false
	}
	if !r.ExpiresAt.IsZero() {
		if now.IsZero() {
			now = time.Now().UTC()
		}
		if now.After(r.ExpiresAt) {
			return false
		}
	}
	if len(r.Operations) == 0 {
		return true
	}
	for _, allowed := range r.Operations {
		if allowed == operation {
			return true
		}
	}
	return false
}
