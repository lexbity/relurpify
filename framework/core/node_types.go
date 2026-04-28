package core

import "context"

// NodePlatform identifies the platform/operating system of a node.
type NodePlatform string

const (
	// NodePlatformMacOS indicates a macOS-based node.
	NodePlatformMacOS NodePlatform = "macos"
	// NodePlatformLinux indicates a Linux-based node.
	NodePlatformLinux NodePlatform = "linux"
	// NodePlatformIOS indicates an iOS-based node.
	NodePlatformIOS NodePlatform = "ios"
	// NodePlatformAndroid indicates an Android-based node.
	NodePlatformAndroid NodePlatform = "android"
	// NodePlatformHeadless indicates a headless node.
	NodePlatformHeadless NodePlatform = "headless"
	// NodePlatformWindows indicates a Windows-based node.
	NodePlatformWindows NodePlatform = "windows"
)

// NodeHealth describes the current state of a connected node.
type NodeHealth struct {
	Online        bool      `json:"online" yaml:"online"`
	BatteryPct    *int      `json:"battery_pct,omitempty" yaml:"battery_pct,omitempty"`
	Charging      *bool     `json:"charging,omitempty" yaml:"charging,omitempty"`
	NetworkType   string    `json:"network_type,omitempty" yaml:"network_type,omitempty"`
	Foreground    bool      `json:"foreground" yaml:"foreground"`
	StorageFreeGB *float64  `json:"storage_free_gb,omitempty" yaml:"storage_free_gb,omitempty"`
	LastSeenAt    int64     `json:"last_seen_at,omitempty" yaml:"last_seen_at,omitempty"`
}

// NodeDescriptor is the stable identity of a device node.
type NodeDescriptor struct {
	ID                   string                 `json:"id" yaml:"id"`
	TenantID             string                 `json:"tenant_id,omitempty" yaml:"tenant_id,omitempty"`
	Name                 string                 `json:"name" yaml:"name"`
	Platform             NodePlatform           `json:"platform" yaml:"platform"`
	TrustClass           TrustClass             `json:"trust_class" yaml:"trust_class"`
	PairedAt             int64                  `json:"paired_at,omitempty" yaml:"paired_at,omitempty"`
	Owner                string                 `json:"owner,omitempty" yaml:"owner,omitempty"`
	Tags                 map[string]string      `json:"tags,omitempty" yaml:"tags,omitempty"`
	ApprovedCapabilities []CapabilityDescriptor `json:"approved_capabilities,omitempty" yaml:"approved_capabilities,omitempty"`
}

// NodeCredential is the pairing credential for a node.
type NodeCredential struct {
	DeviceID  string `json:"device_id" yaml:"device_id"`
	TenantID  string `json:"tenant_id,omitempty" yaml:"tenant_id,omitempty"`
	PublicKey []byte `json:"public_key" yaml:"public_key"`
	KeyID     string `json:"key_id,omitempty" yaml:"key_id,omitempty"`
	IssuedAt  int64  `json:"issued_at" yaml:"issued_at"`
	ExpiresAt int64  `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

// NodeProvider extends Provider for physical device nodes.
type NodeProvider interface {
	Provider
	NodeDescriptor() NodeDescriptor
	NodeHealth(ctx context.Context) (NodeHealth, error)
}

// ProviderCapabilityRegistrar is the interface for registering capabilities.
type ProviderCapabilityRegistrar interface {
	// RegisterCapability registers a capability with the given descriptor.
	RegisterCapability(descriptor CapabilityDescriptor) error
}
