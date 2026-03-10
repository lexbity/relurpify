package core

import "time"

// AdminTokenRecord is the persistent metadata for a runtime-issued admin token.
type AdminTokenRecord struct {
	ID         string     `json:"id" yaml:"id"`
	Name       string     `json:"name,omitempty" yaml:"name,omitempty"`
	SubjectID  string     `json:"subject_id,omitempty" yaml:"subject_id,omitempty"`
	TokenHash  string     `json:"-" yaml:"token_hash"`
	Scopes     []string   `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	IssuedAt   time.Time  `json:"issued_at" yaml:"issued_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty" yaml:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty" yaml:"revoked_at,omitempty"`
}
