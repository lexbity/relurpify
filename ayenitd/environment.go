package ayenitd

import (
	"database/sql"

	"codeburg.org/lexbit/relurpify/framework/ast"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/guidance"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/plan"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	fsandbox "codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/framework/search"
)

// WorkspaceEnvironment is the set of pre-initialized services shared across all
// agents in a workspace session. It is produced by ayenitd.Open() and passed
// directly to agent constructors. It is shallow-copyable; agents may narrow
// scope (e.g. replace Registry for a child execution) without rebuilding.
type WorkspaceEnvironment struct {
	// Identity + model
	Config        *core.Config
	Model         core.LanguageModel
	CommandPolicy fsandbox.CommandPolicy
	Backend       string

	// Capability + permission
	Registry          *capability.Registry
	PermissionManager *fauthorization.PermissionManager

	// Code intelligence
	IndexManager *ast.IndexManager
	SearchEngine *search.SearchEngine

	// Memory + storage
	Memory          memory.MemoryStore
	WorkflowStore   memory.WorkflowStateStore
	CheckpointStore any
	PlanStore       plan.PlanStore
	KnowledgeStore  memory.KnowledgeStore
	GuidanceBroker  *guidance.GuidanceBroker

	// Retrieval
	Embedder    retrieval.Embedder // generic interface, not Ollama-specific
	RetrievalDB *sql.DB            // shared DB for retrieval index tables

	// Agents that verify or extract compatibility surface (optional).
	// These use the agentenv type aliases defined in agentenv_interfaces.go.
	VerificationPlanner           VerificationPlanner
	CompatibilitySurfaceExtractor CompatibilitySurfaceExtractor

	// Scheduler
	Scheduler *ServiceScheduler

	// Service management (new for dynamic lifecycle)
	ServiceManager *ServiceManager
	BKCEvents      *knowledge.EventBus
}

// WithRegistry returns a shallow copy with Registry replaced.
// Agents use this to scope capability access for child executions.
func (e WorkspaceEnvironment) WithRegistry(r *capability.Registry) WorkspaceEnvironment {
	e.Registry = r
	return e
}

// WithMemory returns a shallow copy with Memory replaced.
func (e WorkspaceEnvironment) WithMemory(m memory.MemoryStore) WorkspaceEnvironment {
	e.Memory = m
	return e
}

// WithService adds a service to the ServiceManager via manager.Add().
// This is useful for registering dynamic services at runtime.
func (e WorkspaceEnvironment) WithService(id string, s Service) WorkspaceEnvironment {
	if e.ServiceManager == nil {
		return e
	}
	e.ServiceManager.Register(id, s)
	return e
}
