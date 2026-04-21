package agents

import (
	"database/sql"

	architectpkg "codeburg.org/lexbit/relurpify/agents/architect"
	blackboardpkg "codeburg.org/lexbit/relurpify/agents/blackboard"
	chainerpkg "codeburg.org/lexbit/relurpify/agents/chainer"
	goalconpkg "codeburg.org/lexbit/relurpify/agents/goalcon"
	htnpkg "codeburg.org/lexbit/relurpify/agents/htn"
	pipelinepkg "codeburg.org/lexbit/relurpify/agents/pipeline"
	plannerpkg "codeburg.org/lexbit/relurpify/agents/planner"
	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	reflectionpkg "codeburg.org/lexbit/relurpify/agents/reflection"
	relurpicpkg "codeburg.org/lexbit/relurpify/agents/relurpic"
	rewoopkg "codeburg.org/lexbit/relurpify/agents/rewoo"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/guidance"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

// PlannerAgent re-exports the pattern-based planner so existing callers can
// continue instantiating it via the agents package.
type PlannerAgent = plannerpkg.PlannerAgent

// ReActAgent re-exports the ReAct agent implementation.
type ReActAgent = reactpkg.ReActAgent

// ReflectionAgent re-exports the reviewer agent.
type ReflectionAgent = reflectionpkg.ReflectionAgent

// ModeRuntimeProfile exposes the pattern runtime profile struct.
type ModeRuntimeProfile = reactpkg.ModeRuntimeProfile

// ContextPreferences exposes context tuning knobs.
type ContextPreferences = reactpkg.ContextPreferences

// ArchitectAgent re-exports the architect workflow implementation.
type ArchitectAgent = architectpkg.ArchitectAgent

// WorkflowPlanningService re-exports the architect planning service.
type WorkflowPlanningService = architectpkg.WorkflowPlanningService

// WorkflowPlanningResult re-exports the architect planning result payload.
type WorkflowPlanningResult = architectpkg.WorkflowPlanningResult

// PipelineAgent re-exports the typed pipeline implementation.
type PipelineAgent = pipelinepkg.PipelineAgent

// PipelineStageFactory re-exports the stage factory contract.
type PipelineStageFactory = pipelinepkg.PipelineStageFactory

// AgentInvocationPolicy re-exports composition policy state.
type AgentInvocationPolicy = core.AgentInvocationPolicy

// SQLitePipelineCheckpointStore re-exports the workflow-backed checkpoint store.
type SQLitePipelineCheckpointStore = pipelinepkg.SQLitePipelineCheckpointStore
type RelurpicOption = relurpicpkg.RelurpicOption

var ErrPipelineCheckpointNotFound = pipelinepkg.ErrPipelineCheckpointNotFound

func NewSQLitePipelineCheckpointStore(store *db.SQLiteWorkflowStateStore, workflowID, runID string) *SQLitePipelineCheckpointStore {
	return pipelinepkg.NewSQLitePipelineCheckpointStore(store, workflowID, runID)
}

func RegisterBuiltinRelurpicCapabilities(registry *capability.Registry, model core.LanguageModel, cfg *core.Config) error {
	return relurpicpkg.RegisterBuiltinRelurpicCapabilities(registry, model, cfg)
}

func RegisterBuiltinRelurpicCapabilitiesWithOptions(registry *capability.Registry, model core.LanguageModel, cfg *core.Config, opts ...relurpicpkg.RelurpicOption) error {
	return relurpicpkg.RegisterBuiltinRelurpicCapabilities(registry, model, cfg, opts...)
}
func WithIndexManager(manager *ast.IndexManager) RelurpicOption {
	return relurpicpkg.WithIndexManager(manager)
}
func WithGraphDB(engine *graphdb.Engine) RelurpicOption { return relurpicpkg.WithGraphDB(engine) }
func WithPlanStore(store frameworkplan.PlanStore) RelurpicOption {
	return relurpicpkg.WithPlanStore(store)
}
func WithRetrievalDB(db *sql.DB) RelurpicOption { return relurpicpkg.WithRetrievalDB(db) }
func WithGuidanceBroker(broker *guidance.GuidanceBroker) RelurpicOption {
	return relurpicpkg.WithGuidanceBroker(broker)
}
func WithWorkflowStore(store memory.WorkflowStateStore) RelurpicOption {
	return relurpicpkg.WithWorkflowStore(store)
}

func RegisterAgentCapabilities(registry *capability.Registry, env agentenv.AgentEnvironment) error {
	return relurpicpkg.RegisterAgentCapabilities(registry, env)
}

func RegisterCustomAgentHandler(registry *capability.Registry, id string, handler core.InvocableCapabilityHandler) error {
	return relurpicpkg.RegisterCustomAgentHandler(registry, id, handler)
}

// HTNAgent re-exports the hierarchical task network implementation.
type HTNAgent = htnpkg.HTNAgent

// MethodLibrary re-exports the HTN method registry.
type MethodLibrary = htnpkg.MethodLibrary

// BlackboardAgent re-exports the blackboard architecture implementation.
type BlackboardAgent = blackboardpkg.BlackboardAgent

// RewooAgent re-exports the ReWOO execution implementation.
type RewooAgent = rewoopkg.RewooAgent

// RewooOptions re-exports ReWOO runtime options.
type RewooOptions = rewoopkg.RewooOptions

// RewooPlan re-exports the ReWOO planner output type.
type RewooPlan = rewoopkg.RewooPlan

// ChainerAgent re-exports the isolated chain implementation.
type ChainerAgent = chainerpkg.ChainerAgent

// Chain re-exports the chainer sequence type.
type Chain = chainerpkg.Chain

// Link re-exports the chainer link type.
type Link = chainerpkg.Link

// GoalConAgent re-exports the deterministic backward-chaining implementation.
type GoalConAgent = goalconpkg.GoalConAgent

// GoalCondition re-exports the goal condition type.
type GoalCondition = goalconpkg.GoalCondition

// OperatorRegistry re-exports the goal operator registry.
type OperatorRegistry = goalconpkg.OperatorRegistry

// Operator re-exports the goal operator type.
type Operator = goalconpkg.Operator
