package agentenv

import (
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	cfgsecurity "codeburg.org/lexbit/relurpify/framework/cfgload/security"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

// WorkspaceConfig provides configuration for workspace environment construction.
// This is extracted from ayenitd.WorkspaceConfig to avoid ayenitd dependency
// when using framework/agentenv as the composition root.
type WorkspaceConfig struct {
	Workspace                  string
	ManifestPath               string
	ConfigPath                 string
	StateDir                   string
	InferenceProvider          string
	InferenceEndpoint          string
	InferenceModel             string
	InferenceNativeToolCalling bool
	AgentName                  string
	AgentsDir                  string
	SandboxBackend             string
	AuditLimit                 int
	HITLTimeout                time.Duration
	LogPath                    string
	TelemetryPath              string
	EventsPath                 string
	MemoryPath                 string
	SkipASTIndex               bool
	MaxIterations              int
	AllowedCapabilities        []agentspec.CapabilitySelector
	DebugLLM                   bool
	DebugAgent                 bool
	Strict                     bool
	// Agent specification for policy engine and capability registration
	AgentSpec *agentspec.AgentRuntimeSpec
	// Permission manager for authorization
	PermissionManager *fauthorization.PermissionManager
	// Agent ID for permission tracking
	AgentID string
	// Telemetry for instrumentation
	Telemetry core.Telemetry
	// LoadedConfig contains the resolved config tree produced by cfgload.
	LoadedConfig *cfgload.AppConfig
	// ManifestSnapshot contains the selected agent manifest snapshot.
	ManifestSnapshot *cfgload.AgentManifestSnapshot
	// ProfileResolution is the pre-resolved model profile selected by the caller.
	ProfileResolution llm.ProfileResolution
	// AgentDefinitions are the loaded agent definition overlays selected by the caller.
	AgentDefinitions map[string]*agentspec.AgentDefinition
	// SecurityBundle contains the loaded security policy bundle.
	SecurityBundle *cfgsecurity.Bundle
	// EventLogFactory creates an event.Log implementation for the workspace.
	// If nil, no event log is created. This allows apps to inject app-specific
	// event log implementations (e.g., app/nexus/db) without framework dependencies.
	EventLogFactory func(path string) (event.Log, error)
	// Scope declares which optional feature layers are assembled.
	// A zero value defaults to ScopeFull for backward compatibility.
	Scope WorkspaceScope
}

// InferenceProviderValue returns the inference provider
func (cfg WorkspaceConfig) InferenceProviderValue() string {
	return cfg.InferenceProvider
}

// InferenceEndpointValue returns the inference endpoint
func (cfg WorkspaceConfig) InferenceEndpointValue() string {
	return cfg.InferenceEndpoint
}

// InferenceModelValue returns the inference model
func (cfg WorkspaceConfig) InferenceModelValue() string {
	return cfg.InferenceModel
}

// InferenceNativeToolCallingValue returns whether native tool calling is enabled
func (cfg WorkspaceConfig) InferenceNativeToolCallingValue() bool {
	return cfg.InferenceNativeToolCalling
}




