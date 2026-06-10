package security

import "fmt"

// PolicyRule is a local copy of governance/policy.PolicyRule used for YAML
// loading and validation in the security bundle. The full governance type is
// not imported here to avoid a userconfig→governance import (P15 retired in
// Slice 8). Only the YAML-tagged fields needed for ingestion are mirrored.
type PolicyRule struct {
	ID         string           `yaml:"id"`
	Name       string           `yaml:"name"`
	Priority   int              `yaml:"priority"`
	Enabled    bool             `yaml:"enabled"`
	Conditions PolicyConditions `yaml:"conditions"`
	Effect     PolicyEffect     `yaml:"effect"`
}

// PolicyConditions mirrors governance/policy.PolicyConditions.
type PolicyConditions struct {
	Actors []ActorMatch `yaml:"actors,omitempty"`
}

// ActorMatch mirrors governance/policy.ActorMatch.
type ActorMatch struct {
	ID string `yaml:"id"`
}

// PolicyEffect mirrors governance/policy.PolicyEffect.
type PolicyEffect struct {
	Action      string   `yaml:"action"`
	Approvers   []string `yaml:"approvers,omitempty"`
	ApprovalTTL string   `yaml:"approval_ttl,omitempty"`
	Reason      string   `yaml:"reason,omitempty"`
}

// Validate validates the policy rule locally without importing governance.
func (r PolicyRule) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("policy rule missing id")
	}
	return nil
}
