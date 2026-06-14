package session

import (
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	"codeburg.org/lexbit/relurpify/context/knowledge/search"
	"codeburg.org/lexbit/relurpify/execution/compiler"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/telemetry/event"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	cfgsecurity "codeburg.org/lexbit/relurpify/userconfig/config/security"
	"codeburg.org/lexbit/relurpify/userconfig/modelselect"
)

// Registration is the app-composed runtime registration view consumed by session assembly.
type Registration struct {
	ID          string
	AgentSpec   *agentspec.AgentRuntimeSpec
	Permissions permissions.PermissionManager
	Policy      registry.PolicyEngine
	Audit       policy.AuditLogger
	HITL        any
}

// CapabilityProduct is the app-composed capability product consumed by session assembly.
// The app composition root (app/envcomposition) constructs this product via
// BuildCapabilityRuntime; session receives it for agent runtime
// assembly and workspace session lifecycle.
type CapabilityProduct struct {
	Registry     *registry.CapabilityRegistry
	IndexManager *ast.IndexManager
	SearchEngine *search.SearchEngine
}

// KnowledgeProduct is the app-composed knowledge/retrieval/compilation product.
// The app composition root (app/envcomposition) constructs this product via
// BuildKnowledgeRuntime; session receives it for workspace assembly.
type KnowledgeProduct struct {
	KnowledgeStore  *knowledge.ChunkStore
	KnowledgeEvents *knowledge.EventBus
	Retriever       *retrieval.Retriever
	Compiler        *compiler.Compiler
	StreamTrigger   *contextstream.Trigger
}

// CompiledPolicy captures policy metadata produced during agent bootstrap.
type CompiledPolicy struct {
	AgentID string
	Spec    any
	Rules   []policy.PolicyRule
	Engine  registry.PolicyEngine
}

// WorkspaceConfig provides configuration for workspace environment construction.
// This is extracted from ayenitd.WorkspaceConfig to avoid ayenitd dependency
// while app/envcomposition takes over concrete app wiring.
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
	PermissionManager permissions.PermissionManager
	// Registration contains the app-composed agent runtime registration.
	Registration *Registration
	// Agent ID for permission tracking
	AgentID string
	// Telemetry for instrumentation
	Telemetry telemetry.Telemetry
	// LoadedConfig contains the resolved config tree produced by config.
	LoadedConfig *config.AppConfig
	// DocumentSnapshot contains the loaded document used for contract resolution.
	// When Contract is provided, the document is used only for metadata/fingerprint.
	DocumentSnapshot *config.DocumentSnapshot
	// Contract is a pre-resolved effective agent contract. When non-nil,
	// BootstrapAgentRuntime uses it directly instead of resolving from
	// DocumentSnapshot. S2: euclo uses the built-in contract.
	Contract *config.EffectiveAgentContract
	// ProfileResolution is the pre-resolved model profile selected by the caller.
	ProfileResolution modelselect.ProfileResolution
	// SecurityBundle contains the loaded security policy bundle.
	SecurityBundle *cfgsecurity.Bundle
	// SecurityRuntime contains the app-composed authorized command runner and
	// policy product. When supplied, OpenWorkspace does not build sandbox
	// runner wiring itself.
	SecurityRuntime *RuntimeSecurity
	// CapabilityProduct contains the app-composed capability runtime product.
	// When supplied, BootstrapAgentRuntime does not build the capability bundle
	// itself — it uses the pre-built registry, index manager, and search engine.
	CapabilityProduct *CapabilityProduct
	// KnowledgeProduct contains the app-composed knowledge, retrieval, compiler,
	// and stream trigger product.
	KnowledgeProduct *KnowledgeProduct
	// ModelProduct is the pre-built model runtime. The app composition root
	// (app/envcomposition) constructs provider backends and instrumentation.
	ModelProduct *model.ModelProduct
	// EventLogFactory creates an event.Log implementation for the workspace.
	// If nil, no event log is created. This allows apps to inject app-specific
	// event log implementations (e.g., app/nexus/db) without framework dependencies.
	EventLogFactory func(path string) (event.Log, error)
	// Scope declares which optional feature layers are assembled.
	// A zero value defaults to ScopeFull.
	Scope WorkspaceScope
}

// RuntimeSecurity is the security product consumed by OpenWorkspace.
//
// The app composition root owns construction of this product. execution/agentenv
// only consumes it while the environment object is being dissolved.
type RuntimeSecurity struct {
	Runner        *sandbox.AuthorizedRunner
	PolicyEngine  registry.PolicyEngine
	CommandPolicy sandbox.CommandPolicy
	Permissions   permissions.PermissionManager
	RunnerConfig  *sandbox.CommandRunnerConfig
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
