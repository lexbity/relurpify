package core

import (
	"errors"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

// DelegationSubjectRef identifies a subject granted delegation permissions.
type DelegationSubjectRef struct {
	Kind       string `json:"kind" yaml:"kind"`
	ID         string `json:"id" yaml:"id"`
	ProviderID string `json:"provider_id,omitempty" yaml:"provider_id,omitempty"`
	TenantID   string `json:"tenant_id" yaml:"tenant_id"`
}

func (s DelegationSubjectRef) Validate() error {
	if strings.TrimSpace(s.Kind) == "" {
		return errors.New("subject kind required")
	}
	if strings.TrimSpace(s.ID) == "" {
		return errors.New("subject id required")
	}
	if strings.TrimSpace(s.TenantID) == "" {
		return errors.New("subject tenant_id required")
	}
	return nil
}

func (s DelegationSubjectRef) Matches(actor identity.EventActor) bool {
	// Match kind case-insensitively
	if !strings.EqualFold(s.Kind, actor.Kind) && !strings.EqualFold(s.Kind, actor.SubjectKind) {
		return false
	}

	// Match ID exactly
	if s.ID != actor.ID {
		return false
	}

	// If TenantID is set on subject, match against actor's TenantID
	// Fall back to actor.ID if actor.TenantID is empty
	if s.TenantID != "" {
		actorTenantID := actor.TenantID
		if actorTenantID == "" {
			actorTenantID = actor.ID
		}
		if s.TenantID != actorTenantID {
			return false
		}
	}

	return true
}

// SessionDelegationRecord grants a tenant-scoped subject permission to act on a
// session they do not own for specific session operations.
type SessionDelegationRecord struct {
	TenantID   string               `json:"tenant_id" yaml:"tenant_id"`
	SessionID  string               `json:"session_id" yaml:"session_id"`
	Grantee    DelegationSubjectRef `json:"grantee" yaml:"grantee"`
	Operations []SessionOperation   `json:"operations,omitempty" yaml:"operations,omitempty"`
	CreatedAt  time.Time            `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	ExpiresAt  time.Time            `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

func (r SessionDelegationRecord) Validate() error {
	if strings.TrimSpace(r.TenantID) == "" {
		return errors.New("session delegation tenant_id required")
	}
	if strings.TrimSpace(r.SessionID) == "" {
		return errors.New("session delegation session_id required")
	}
	if err := r.Grantee.Validate(); err != nil {
		return err
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

func (r SessionDelegationRecord) Allows(actor identity.EventActor, operation SessionOperation, now time.Time) bool {
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
