package agents

import (
	architectpkg "github.com/lexcodex/relurpify/agents/architect"
	blackboardpkg "github.com/lexcodex/relurpify/agents/blackboard"
	chainerpkg "github.com/lexcodex/relurpify/agents/chainer"
	goalconpkg "github.com/lexcodex/relurpify/agents/goalcon"
	htnpkg "github.com/lexcodex/relurpify/agents/htn"
	pipelinepkg "github.com/lexcodex/relurpify/agents/pipeline"
	plannerpkg "github.com/lexcodex/relurpify/agents/planner"
	reactpkg "github.com/lexcodex/relurpify/agents/react"
	reflectionpkg "github.com/lexcodex/relurpify/agents/reflection"
	relurpicpkg "github.com/lexcodex/relurpify/agents/relurpic"
	rewoopkg "github.com/lexcodex/relurpify/agents/rewoo"
	skillspkg "github.com/lexcodex/relurpify/agents/skills"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/memory/db"
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

// SkillPaths exposes the resolved resource paths for a skill.
type SkillPaths = skillspkg.SkillPaths

// SkillResolution captures skill loading outcomes.
type SkillResolution = skillspkg.SkillResolution

// ResolvedSkill carries validated manifests and resolved paths for later
// registration after pure resolution completes.
type ResolvedSkill = skillspkg.ResolvedSkill

// SkillCapabilityCandidate represents a prompt/resource capability contributed
// by a resolved skill.
type SkillCapabilityCandidate = skillspkg.SkillCapabilityCandidate

var ErrPipelineCheckpointNotFound = pipelinepkg.ErrPipelineCheckpointNotFound

func NewSQLitePipelineCheckpointStore(store *db.SQLiteWorkflowStateStore, workflowID, runID string) *SQLitePipelineCheckpointStore {
	return pipelinepkg.NewSQLitePipelineCheckpointStore(store, workflowID, runID)
}

func RegisterBuiltinRelurpicCapabilities(registry *capability.Registry, model core.LanguageModel, cfg *core.Config) error {
	return relurpicpkg.RegisterBuiltinRelurpicCapabilities(registry, model, cfg)
}

func RegisterAgentCapabilities(registry *capability.Registry, env agentenv.AgentEnvironment) error {
	return relurpicpkg.RegisterAgentCapabilities(registry, env)
}

func RegisterCustomAgentHandler(registry *capability.Registry, id string, handler core.InvocableCapabilityHandler) error {
	return relurpicpkg.RegisterCustomAgentHandler(registry, id, handler)
}

func ResolveSkillPaths(skill *manifest.SkillManifest) SkillPaths {
	return skillspkg.ResolveSkillPaths(skill)
}

func ValidateSkillPaths(paths SkillPaths) error {
	return skillspkg.ValidateSkillPaths(paths)
}

func DeriveGVisorAllowlist(allowed []core.CapabilitySelector, registry skillspkg.ToolDescriptorRegistry) []core.ExecutablePermission {
	return skillspkg.DeriveGVisorAllowlist(allowed, registry)
}

func ApplySkills(workspace string, baseSpec *core.AgentRuntimeSpec, skillNames []string,
	registry *capability.Registry, permissions *capability.PermissionManager, agentID string,
) (*core.AgentRuntimeSpec, []SkillResolution) {
	return skillspkg.ApplySkills(workspace, baseSpec, skillNames, registry, permissions, agentID)
}

func ResolveSkills(workspace string, baseSpec *core.AgentRuntimeSpec, skillNames []string) (*core.AgentRuntimeSpec, []ResolvedSkill, []SkillResolution) {
	return skillspkg.ResolveSkills(workspace, baseSpec, skillNames)
}

func EnumerateSkillCapabilities(resolved []ResolvedSkill) []SkillCapabilityCandidate {
	return skillspkg.EnumerateSkillCapabilities(resolved)
}

func SkillRoot(workspace, name string) string {
	return skillspkg.SkillRoot(workspace, name)
}

func SkillManifestPath(workspace, name string) string {
	return skillspkg.SkillManifestPath(workspace, name)
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
