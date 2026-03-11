package core

import (
	"errors"
	"strings"
	"time"
)

// TenantRecord is the durable metadata for a tenant isolation boundary.
type TenantRecord struct {
	ID          string     `json:"id" yaml:"id"`
	DisplayName string     `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	CreatedAt   time.Time  `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	DisabledAt  *time.Time `json:"disabled_at,omitempty" yaml:"disabled_at,omitempty"`
}

// SubjectRecord is the durable metadata for a tenant-scoped subject.
type SubjectRecord struct {
	TenantID    string      `json:"tenant_id" yaml:"tenant_id"`
	Kind        SubjectKind `json:"kind" yaml:"kind"`
	ID          string      `json:"id" yaml:"id"`
	DisplayName string      `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	Roles       []string    `json:"roles,omitempty" yaml:"roles,omitempty"`
	CreatedAt   time.Time   `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	DisabledAt  *time.Time  `json:"disabled_at,omitempty" yaml:"disabled_at,omitempty"`
}

func (t TenantRecord) Validate() error {
	if strings.TrimSpace(t.ID) == "" {
		return errors.New("tenant id required")
	}
	if t.DisabledAt != nil && !t.CreatedAt.IsZero() && t.DisabledAt.Before(t.CreatedAt) {
		return errors.New("tenant disabled_at must be after created_at")
	}
	return nil
}

func (s SubjectRecord) Validate() error {
	if err := (SubjectRef{TenantID: s.TenantID, Kind: s.Kind, ID: s.ID}).Validate(); err != nil {
		return err
	}
	if s.DisabledAt != nil && !s.CreatedAt.IsZero() && s.DisabledAt.Before(s.CreatedAt) {
		return errors.New("subject disabled_at must be after created_at")
	}
	for _, role := range s.Roles {
		if strings.TrimSpace(role) == "" {
			return errors.New("subject role must not be empty")
		}
	}
	return nil
}
