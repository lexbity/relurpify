package agentenv

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/ast"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/compiler"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
	"codeburg.org/lexbit/relurpify/framework/jobs"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/framework/search"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type VerificationPlanner interface {
	SelectVerificationPlan(context.Context, contracts.VerificationPlanRequest) (contracts.VerificationPlan, bool, error)
}

type CompatibilitySurfaceExtractor interface {
	ExtractSurface(context.Context, contracts.CompatibilitySurfaceRequest) (contracts.CompatibilitySurface, bool, error)
}

// WorkspaceEnvironment is the canonical runtime environment shared by all agents
// in a workspace session. It is produced by ayenitd.Open() and passed directly
// to agent constructors. It is shallow-copyable; agents may narrow scope
// (e.g. replace Registry for a child execution) without rebuilding.
//
// Layering note: WorkspaceEnvironment is assembled exclusively by the
// composition root (ayenitd.Open()) and must not be constructed by platform code.
// Platform packages may receive WorkspaceEnvironment as a dependency but must
// not import framework/agentenv to construct it.
//
// Ownership note: WorkspaceEnvironment is a composition root only. It does not
// define storage models or business logic. Storage concerns are delegated to:
// - framework/compiler for compilation state
// - framework/agentlifecycle for runtime lifecycle state
// - framework/persistence for persistence adapters
// - framework/graphdb for durable backend implementation
type WorkspaceEnvironment struct {
	// Identity + model
	Config        *core.Config
	Model         contracts.LanguageModel
	CommandPolicy sandbox.CommandPolicy
	FileScope     *sandbox.FileScopePolicy
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
	Registry *capability.Registry
	// PermissionManager is the single implementation of the permission manager interface.
	// Kept as concrete type for direct access to permission enforcement methods.
	PermissionManager *fauthorization.PermissionManager

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
	PatternStore patterns.PatternStore
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

	// Optional agents (interfaces)
	VerificationPlanner           VerificationPlanner
	CompatibilitySurfaceExtractor CompatibilitySurfaceExtractor
}

// WithRegistry returns a shallow copy with Registry replaced.
// Agents use this to scope capability access for child executions.
func (e WorkspaceEnvironment) WithRegistry(r *capability.Registry) WorkspaceEnvironment {
	e.Registry = r
	return e
}

// WithMemory returns a shallow copy with WorkingMemory replaced.
func (e WorkspaceEnvironment) WithMemory(m *memory.WorkingMemoryStore) WorkspaceEnvironment {
	e.WorkingMemory = m
	return e
}

// WithCommandRunner returns a shallow copy with CommandRunner replaced.
// Use this in tests to inject a recording or no-op runner without building a
// full sandbox runtime.
func (e WorkspaceEnvironment) WithCommandRunner(r sandbox.CommandRunner) WorkspaceEnvironment {
	e.CommandRunner = r
	return e
}

// WithFileScope returns a shallow copy with FileScope replaced.
func (e WorkspaceEnvironment) WithFileScope(scope *sandbox.FileScopePolicy) WorkspaceEnvironment {
	e.FileScope = scope
	return e
}

// WithJobSubmitter returns a shallow copy with JobSubmitter replaced.
func (e WorkspaceEnvironment) WithJobSubmitter(s jobs.Submitter) WorkspaceEnvironment {
	e.JobSubmitter = s
	return e
}

// WithService adds a service to the ServiceManager via manager.Register().
// This is useful for registering dynamic services at runtime.
func (e WorkspaceEnvironment) WithService(id string, s Service) WorkspaceEnvironment {
	if e.ServiceManager == nil {
		return e
	}
	if e.ServiceManager.Registry == nil {
		e.ServiceManager.Registry = make(map[string]Service)
	}
	e.ServiceManager.Registry[id] = s
	return e
}
