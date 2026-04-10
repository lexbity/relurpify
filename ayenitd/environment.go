package ayenitd

import (
	"database/sql"

	archaeobkc "github.com/lexcodex/relurpify/archaeo/bkc"
	"github.com/lexcodex/relurpify/framework/ast"
	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/patterns"
	"github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/framework/retrieval"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/lexcodex/relurpify/framework/search"
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
	CheckpointStore *memory.CheckpointStore // nil until implemented in framework
	PlanStore       plan.PlanStore
	PatternStore    patterns.PatternStore
	CommentStore    patterns.CommentStore
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
	BKCEvents      *archaeobkc.EventBus
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
