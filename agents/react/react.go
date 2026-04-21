package react

import (
	"context"
	"log"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextmgr"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	frameworksearch "codeburg.org/lexbit/relurpify/framework/search"
)

// ReActAgent implements the Reason+Act pattern.
// ModeRuntimeProfile conveys high-level runtime settings to the agent.
type ModeRuntimeProfile struct {
	Name        string
	Description string
	Temperature float64
	Context     ContextPreferences
}

// ContextPreferences tune context management for a mode.
type ContextPreferences struct {
	PreferredDetailLevel contextmgr.DetailLevel
	MinHistorySize       int
	CompressionThreshold float64
}

// ReActAgent implements the Reason+Act pattern.
type ReActAgent struct {
	Model          core.LanguageModel
	Tools          *capability.Registry
	Memory         memory.MemoryStore
	Config         *core.Config
	IndexManager   *ast.IndexManager
	SearchEngine   *frameworksearch.SearchEngine
	Summarizer     core.Summarizer
	CheckpointPath string
	maxIterations  int
	contextPolicy  *contextmgr.ContextPolicy

	Mode             string
	ModeProfile      ModeRuntimeProfile
	sharedContext    *core.SharedContext
	initialLoadDone  bool
	executionCatalog *capability.ExecutionCapabilityCatalogSnapshot

	// SemanticContext is the pre-resolved semantic context bundle passed
	// to the agent at construction time. Chunks are injected into the
	// context window before InitialLoad runs.
	SemanticContext core.AgentSemanticContext
}

const (
	contextmgrPhaseExplore = "explore"
	contextmgrPhaseEdit    = "edit"
	contextmgrPhaseVerify  = "verify"
)

// Initialize wires configuration.
func (a *ReActAgent) Initialize(config *core.Config) error {
	a.Config = config
	if config.MaxIterations <= 0 {
		a.maxIterations = defaultIterationsForMode(a.Mode)
	} else {
		a.maxIterations = config.MaxIterations
	}
	if a.Tools == nil {
		a.Tools = capability.NewRegistry()
	}
	if a.Mode == "" {
		a.Mode = "code"
	}
	if a.ModeProfile.Name == "" {
		a.ModeProfile = ModeRuntimeProfile{
			Name:        a.Mode,
			Description: "Reason + Act agent",
			Temperature: 0.2,
			Context: ContextPreferences{
				PreferredDetailLevel: contextmgr.DetailDetailed,
				MinHistorySize:       5,
				CompressionThreshold: 0.8,
			},
		}
	}
	strategy := contextmgr.ContextStrategy(nil)
	if a.contextPolicy != nil {
		strategy = a.contextPolicy.Strategy
	}
	if strategy == nil {
		switch strings.ToLower(a.Mode) {
		case "debug", "ask":
			strategy = contextmgr.NewAggressiveStrategy()
		case "architect":
			strategy = contextmgr.NewConservativeStrategy()
		default:
			strategy = contextmgr.NewAdaptiveStrategy()
		}
	}
	var spec *core.AgentContextSpec
	if config != nil && config.AgentSpec != nil {
		spec = &config.AgentSpec.Context
	}
	if a.contextPolicy == nil {
		a.contextPolicy = contextmgr.NewContextPolicy(contextmgr.ContextPolicyConfig{
			Strategy:      strategy,
			LanguageModel: a.Model,
			IndexManager:  a.IndexManager,
			SearchEngine:  a.SearchEngine,
			MemoryStore:   a.Memory,
			Summarizer:    a.Summarizer,
			Preferences: contextmgr.ContextPolicyPreferences{
				PreferredDetailLevel: a.ModeProfile.Context.PreferredDetailLevel,
				MinHistorySize:       a.ModeProfile.Context.MinHistorySize,
				CompressionThreshold: a.ModeProfile.Context.CompressionThreshold,
			},
		}, spec)
	} else {
		a.contextPolicy.Strategy = strategy
		a.contextPolicy.Preferences = contextmgr.ContextPolicyPreferences{
			PreferredDetailLevel: a.ModeProfile.Context.PreferredDetailLevel,
			MinHistorySize:       a.ModeProfile.Context.MinHistorySize,
			CompressionThreshold: a.ModeProfile.Context.CompressionThreshold,
		}
		a.contextPolicy.ApplyAgentContextSpec(spec)
	}
	// Inject pre-computed chunks from semantic context before InitialLoad runs
	if a.contextPolicy != nil && a.contextPolicy.Progressive != nil && len(a.SemanticContext.Chunks) > 0 {
		chunks := convertAgentChunksToContextChunks(a.SemanticContext.Chunks)
		_ = a.contextPolicy.Progressive.InjectPrecomputedChunks(chunks)
	}
	a.contextPolicy.Budget.SetReservations(1000, 2000, 1000)
	return nil
}

// convertAgentChunksToContextChunks converts core.AgentContextChunk to contextmgr.ContextChunk
func convertAgentChunksToContextChunks(agentChunks []core.AgentContextChunk) []contextmgr.ContextChunk {
	chunks := make([]contextmgr.ContextChunk, len(agentChunks))
	for i, ac := range agentChunks {
		chunks[i] = contextmgr.ContextChunk{
			ID:            ac.ID,
			Content:       ac.Content,
			TokenEstimate: ac.TokenEstimate,
			Metadata:      ac.Metadata,
		}
	}
	return chunks
}

// debugf logs formatted messages whenever agent debug logging is enabled.
func (a *ReActAgent) debugf(format string, args ...interface{}) {
	if a == nil || a.Config == nil || !a.Config.DebugAgent {
		return
	}
	log.Printf("[react] "+format, args...)
}

// Execute runs the task through the workflow graph.
func (a *ReActAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	a.executionCatalog = nil
	if a.Tools != nil {
		a.executionCatalog = a.Tools.CaptureExecutionCatalogSnapshot()
	}
	g, err := a.BuildGraph(task)
	if err != nil {
		a.executionCatalog = nil
		return nil, err
	}
	a.initialLoadDone = false
	a.sharedContext = core.NewSharedContext(state, a.contextPolicy.Budget, a.contextPolicy.Summarizer)
	if a.contextPolicy != nil && task != nil {
		if err := a.contextPolicy.InitialLoad(task); err != nil {
			a.debugf("initial context load failed: %v", err)
		} else {
			a.initialLoadDone = true
		}
	}
	defer func() {
		a.sharedContext = nil
		a.initialLoadDone = false
		a.executionCatalog = nil
	}()
	if cfg := a.Config; cfg != nil && cfg.Telemetry != nil {
		g.SetTelemetry(cfg.Telemetry)
	}
	a.initializePhase(state, task)
	if !reactUsesExplicitCheckpointNodes(a.Config) && a.CheckpointPath != "" && task != nil && task.ID != "" {
		store := memory.NewCheckpointStore(filepath.Clean(a.CheckpointPath))
		g.WithCheckpointing(2, store.Save)
	}
	result, err := g.Execute(ctx, state)
	return a.finalizeExecuteResult(ctx, task, state, result, err)
}

// Capabilities describes what the agent can do.
func (a *ReActAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityCode,
		core.CapabilityExplain,
	}
}
