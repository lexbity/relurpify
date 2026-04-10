package agentenv

import (
	"context"

	"github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/lexcodex/relurpify/framework/search"
)

type VerificationCommand struct {
	Name             string
	Command          string
	Args             []string
	WorkingDirectory string
}

type VerificationPlan struct {
	ScopeKind              string
	Files                  []string
	TestFiles              []string
	Commands               []VerificationCommand
	Source                 string
	PlannerID              string
	Rationale              string
	AuditTrail             []string
	CompatibilitySensitive bool
	Metadata               map[string]any
}

type VerificationPlanRequest struct {
	TaskInstruction                 string
	ModeID                          string
	ProfileID                       string
	Workspace                       string
	Files                           []string
	TestFiles                       []string
	PublicSurfaceChanged            bool
	PreferredVerifyCapabilities     []string
	VerificationSuccessCapabilities []string
	RequireVerificationStep         bool
}

type VerificationPlanner interface {
	SelectVerificationPlan(context.Context, VerificationPlanRequest) (VerificationPlan, bool, error)
}

type CompatibilitySurface struct {
	Functions []map[string]any
	Types     []map[string]any
	Metadata  map[string]any
}

type CompatibilitySurfaceRequest struct {
	TaskInstruction string
	Workspace       string
	Files           []string
	FileContents    []map[string]any
}

type CompatibilitySurfaceExtractor interface {
	ExtractSurface(context.Context, CompatibilitySurfaceRequest) (CompatibilitySurface, bool, error)
}

// AgentEnvironment bundles the shared runtime dependencies required by agent
// implementations. The container is shallow-copyable so callers can scope
// registry or memory access for child executions without rebuilding the world.
type AgentEnvironment struct {
	Model                         core.LanguageModel
	Registry                      *capability.Registry
	IndexManager                  *ast.IndexManager
	SearchEngine                  *search.SearchEngine
	Memory                        memory.MemoryStore
	Config                        *core.Config
	CommandPolicy                 sandbox.CommandPolicy
	VerificationPlanner           VerificationPlanner
	CompatibilitySurfaceExtractor CompatibilitySurfaceExtractor
}

// WithRegistry returns a shallow copy with Registry replaced.
func (e AgentEnvironment) WithRegistry(r *capability.Registry) AgentEnvironment {
	e.Registry = r
	return e
}

// WithMemory returns a shallow copy with Memory replaced.
func (e AgentEnvironment) WithMemory(m memory.MemoryStore) AgentEnvironment {
	e.Memory = m
	return e
}
