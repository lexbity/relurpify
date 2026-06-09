package agentenv

import (
	"context"

	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/memory"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	"codeburg.org/lexbit/relurpify/context/knowledge/search"
	"codeburg.org/lexbit/relurpify/context/persistence/artifactstore"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
	"codeburg.org/lexbit/relurpify/execution/compiler"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/jobs"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/telemetry/event"
)

type VerificationPlanner interface {
	SelectVerificationPlan(context.Context, VerificationPlanRequest) (VerificationPlan, bool, error)
}

type CompatibilitySurfaceExtractor interface {
	ExtractSurface(context.Context, CompatibilitySurfaceRequest) (CompatibilitySurface, bool, error)
}

// AgentContext is the canonical runtime environment shared by all agents
// in a workspace session. It is produced by ayenitd.Open() and passed directly
// to agent constructors. It is shallow-copyable; agents may narrow scope
// (e.g. replace Registry for a child execution) without rebuilding.
type AgentContext struct {
	// Identity + model
	Config        *execution.Config
	Model         model.LanguageModel
	CommandPolicy sandbox.CommandPolicy
	FileScope     *permissions.FileScopePolicy
	// CommandRunner is the sandbox-enforced runner built by ayenitd from the
	// manifest-declared command allowlist. Named agents and their capability
	// handlers use this to execute shell, git, and test commands without
	// importing ayenitd. Nil when no sandbox runtime is configured (tests may
	// substitute a local runner or a test double).
	CommandRunner sandbox.CommandRunner
	Backend       string

	// Capability + permission
	// Registry is the single implementation of the capability registry interface.
	// Kept as concrete type for direct access to registration methods.
	Registry *registry.CapabilityRegistry
	// PermissionManager is the single implementation of the permission manager interface.
	// Kept as concrete type for direct access to permission enforcement methods.
	PermissionManager PermissionManager

	// Code intelligence
	// IndexManager is the single implementation of the AST index manager interface.
	// Kept as concrete type for direct access to indexing methods.
	IndexManager *ast.IndexManager
	// SearchEngine is the concrete search engine implementation.
	// Could be extracted to an interface if multiple implementations are needed.
	SearchEngine *search.SearchEngine

	// Knowledge + memory
	// WorkingMemoryStore is the concrete working memory implementation.
	// Kept as concrete type for direct access to memory operations.
	WorkingMemory *memory.WorkingMemoryStore
	// KnowledgeStore is the concrete chunk store implementation.
	// Kept as concrete type for direct access to knowledge operations.
	KnowledgeStore *knowledge.ChunkStore
	// OutputIngester closes the write loop from runtime outputs back into knowledge.
	OutputIngester *knowledge.OutputIngester
	// IngestOutputs enables runtime output ingestion for agents that opt in.
	IngestOutputs bool
	// PatternStore is the pattern store interface.
	PatternStore PatternStore
	// AgentLifecycle is the runtime agent lifecycle management interface.
	// This handles delegation, event, and lineage persistence.
	AgentLifecycle agentlifecycle.Repository

	// Retrieval + compilation
	// Retriever is the concrete retrieval implementation.
	// Kept as concrete type for direct access to retrieval methods.
	Retriever *retrieval.Retriever
	// Compiler is the concrete compiler implementation.
	// Kept as concrete type for direct access to compilation methods.
	Compiler *compiler.Compiler
	// StreamTrigger is the context streaming trigger built from Compiler.
	// Agents inject it into the execution context via contextstream.WithTrigger
	// so StreamTriggerNodes can resolve it without holding it as a field.
	StreamTrigger *contextstream.Trigger

	// PromptRegistry is the workspace-scoped prompt registry. Loaded from
	// relurpify_cfg/prompts/ at Open() time. Named agents register their
	// providers during Initialize(). Nil in test environments that do not
	// exercise prompt resolution.
	PromptRegistry prompt.Registry

	// Event infrastructure
	EventLog        event.Log
	KnowledgeEvents *knowledge.EventBus

	// Scheduling + services
	Scheduler      *ServiceScheduler
	ServiceManager *ServiceManager
	// JobSubmitter allows capability handlers and agents to enqueue long-running
	// work into the framework job queue without holding a full JobStore reference.
	// Nil when the workspace is not backed by a persistent job store (e.g., in
	// unit tests). Capabilities must check for nil before calling Submit.
	JobSubmitter jobs.Submitter

	// ArtifactStore provides durable per-session storage for large tool output.
	// Created by OpenWorkspace; GC'd at session end or when size cap is exceeded.
	ArtifactStore artifactstore.Store

	// Optional agents (interfaces)
	VerificationPlanner           VerificationPlanner
	CompatibilitySurfaceExtractor CompatibilitySurfaceExtractor
}

// WithRegistry returns a shallow copy with Registry replaced.
// Agents use this to scope capability access for child executions.
func (e AgentContext) WithRegistry(r *registry.CapabilityRegistry) AgentContext {
	e.Registry = r
	return e
}

// WithMemory returns a shallow copy with WorkingMemory replaced.
func (e AgentContext) WithMemory(m *memory.WorkingMemoryStore) AgentContext {
	e.WorkingMemory = m
	return e
}

// WithCommandRunner returns a shallow copy with CommandRunner replaced.
// Use this in tests to inject a recording or no-op runner without building a
// full sandbox runtime.
func (e AgentContext) WithCommandRunner(r sandbox.CommandRunner) AgentContext {
	e.CommandRunner = r
	return e
}

// WithFileScope returns a shallow copy with FileScope replaced.
func (e AgentContext) WithFileScope(scope *permissions.FileScopePolicy) AgentContext {
	e.FileScope = scope
	return e
}

// WithJobSubmitter returns a shallow copy with JobSubmitter replaced.
func (e AgentContext) WithJobSubmitter(s jobs.Submitter) AgentContext {
	e.JobSubmitter = s
	return e
}

// WithService adds a service to the ServiceManager via manager.Register().
// This is useful for registering dynamic services at runtime.
func (e AgentContext) WithService(id string, s Service) AgentContext {
	if e.ServiceManager == nil {
		return e
	}
	if e.ServiceManager.Registry == nil {
		e.ServiceManager.Registry = make(map[string]Service)
	}
	e.ServiceManager.Registry[id] = s
	return e
}

// WithPromptRegistry returns a shallow copy with PromptRegistry replaced.
func (e AgentContext) WithPromptRegistry(r prompt.Registry) AgentContext {
	e.PromptRegistry = r
	return e
}
