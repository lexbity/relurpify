package session

import (
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

// agentEnv is the internal runtime data shared within a workspace session.
// It is not exported — consumers use session controllers or the Workspace
// struct's individual fields instead.
type agentEnv struct {
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
	PermissionManager permissions.PermissionManager

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
	PatternStore knowledge.PatternStore
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
	ServiceManager ServiceManager
	// JobSubmitter allows capability handlers and agents to enqueue long-running
	// work into the framework job queue without holding a full JobStore reference.
	// Nil when the workspace is not backed by a persistent job store (e.g., in
	// unit tests). Capabilities must check for nil before calling Submit.
	JobSubmitter jobs.Submitter

	// ArtifactStore provides durable per-session storage for large tool output.
	// Created by OpenWorkspace; GC'd at session end or when size cap is exceeded.
	ArtifactStore artifactstore.Store
}
