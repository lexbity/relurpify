package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type NodePlatform string

const (
	NodePlatformMacOS    NodePlatform = "macos"
	NodePlatformLinux    NodePlatform = "linux"
	NodePlatformIOS      NodePlatform = "ios"
	NodePlatformAndroid  NodePlatform = "android"
	NodePlatformHeadless NodePlatform = "headless"
	NodePlatformWindows  NodePlatform = "windows"
)

// NodeDescriptor is the stable identity of a device node.
type NodeDescriptor struct {
	ID         string            `json:"id" yaml:"id"`
	TenantID   string            `json:"tenant_id,omitempty" yaml:"tenant_id,omitempty"`
	Name       string            `json:"name" yaml:"name"`
	Platform   NodePlatform      `json:"platform" yaml:"platform"`
	TrustClass TrustClass        `json:"trust_class" yaml:"trust_class"`
	PairedAt   time.Time         `json:"paired_at,omitempty" yaml:"paired_at,omitempty"`
	Owner      SubjectRef        `json:"owner,omitempty" yaml:"owner,omitempty"`
	Tags       map[string]string `json:"tags,omitempty" yaml:"tags,omitempty"`
}

// NodeHealth describes the current state of a connected node.
type NodeHealth struct {
	Online        bool      `json:"online" yaml:"online"`
	BatteryPct    *int      `json:"battery_pct,omitempty" yaml:"battery_pct,omitempty"`
	Charging      *bool     `json:"charging,omitempty" yaml:"charging,omitempty"`
	NetworkType   string    `json:"network_type,omitempty" yaml:"network_type,omitempty"`
	Foreground    bool      `json:"foreground" yaml:"foreground"`
	StorageFreeGB *float64  `json:"storage_free_gb,omitempty" yaml:"storage_free_gb,omitempty"`
	LastSeenAt    time.Time `json:"last_seen_at,omitempty" yaml:"last_seen_at,omitempty"`
}

// NodeCredential is the pairing credential for a node.
type NodeCredential struct {
	DeviceID  string    `json:"device_id" yaml:"device_id"`
	TenantID  string    `json:"tenant_id,omitempty" yaml:"tenant_id,omitempty"`
	PublicKey []byte    `json:"public_key" yaml:"public_key"`
	KeyID     string    `json:"key_id,omitempty" yaml:"key_id,omitempty"`
	IssuedAt  time.Time `json:"issued_at" yaml:"issued_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

// NodeProvider extends Provider for physical device nodes.
type NodeProvider interface {
	Provider
	NodeDescriptor() NodeDescriptor
	NodeHealth(ctx context.Context) (NodeHealth, error)
	StreamHealth(ctx context.Context) (<-chan NodeHealth, error)
	VerifyCredential(cred NodeCredential) error
}

func (d NodeDescriptor) Validate() error {
	if strings.TrimSpace(d.ID) == "" {
		return errors.New("node id required")
	}
	if strings.TrimSpace(d.Name) == "" {
		return errors.New("node name required")
	}
	switch d.Platform {
	case NodePlatformMacOS, NodePlatformLinux, NodePlatformIOS, NodePlatformAndroid, NodePlatformHeadless, NodePlatformWindows:
	default:
		return fmt.Errorf("node platform %s invalid", d.Platform)
	}
	switch d.TrustClass {
	case TrustClassBuiltinTrusted, TrustClassWorkspaceTrusted, TrustClassProviderLocalUntrusted, TrustClassRemoteDeclared, TrustClassRemoteApproved:
	default:
		return fmt.Errorf("trust class %s invalid", d.TrustClass)
	}
	if d.Owner.ID != "" {
		if err := d.Owner.Validate(); err != nil {
			return fmt.Errorf("owner invalid: %w", err)
		}
		if d.TenantID != "" && !strings.EqualFold(d.TenantID, d.Owner.TenantID) {
			return errors.New("node tenant_id must match owner tenant_id")
		}
	}
	return nil
}

func (c NodeCredential) Validate() error {
	if strings.TrimSpace(c.DeviceID) == "" {
		return errors.New("device id required")
	}
	if len(c.PublicKey) == 0 {
		return errors.New("public key required")
	}
	if c.IssuedAt.IsZero() {
		return errors.New("issued_at required")
	}
	if !c.ExpiresAt.IsZero() && c.ExpiresAt.Before(c.IssuedAt) {
		return errors.New("expires_at must be after issued_at")
	}
	return nil
}
