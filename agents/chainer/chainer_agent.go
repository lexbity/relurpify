package chainer

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/agents/chainer/checkpoint"
	chainctx "github.com/lexcodex/relurpify/agents/chainer/context"
	"github.com/lexcodex/relurpify/agents/chainer/telemetry"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/pipeline"
)

// ChainerAgent executes a deterministic chain of isolated LLM links.
//
// # Phase 1: Legacy Execution (Original)
//
// Execute() uses the custom chainRunner (runner.go) for backward compatibility.
//
// # Phase 2: Pipeline-Based Execution (New, Optional)
//
// When CheckpointStore is configured, Execute() uses framework/pipeline.Runner
// for resumable, checkpointed execution. Links are converted to pipeline.Stage
// objects, enabling:
//   - Interruption-safe execution (resume from last completed stage)
//   - Automatic input isolation via Stage contracts
//   - Optional telemetry and context budget management (later phases)
type ChainerAgent struct {
	Model                 core.LanguageModel
	Tools                 *capability.Registry
	Memory                memory.MemoryStore
	Config                *core.Config
	Chain                 *Chain
	ChainBuilder          func(*core.Task) (*Chain, error)
	CheckpointStore       pipeline.CheckpointStore // Phase 2: Optional checkpoint store
	CheckpointAfterStage  bool                      // Phase 2: Save checkpoint after each stage
	RecoveryManager       *checkpoint.RecoveryManager // Phase 2: Manages resumption
	BudgetManager         *chainctx.SimpleBudgetTracker // Phase 3: Optional token budget tracking
	CompressionListener   *chainctx.CompressionListener // Phase 3: Reacts to budget events
	EventRecorder         *telemetry.EventRecorder // Phase 4: Optional telemetry recording
	initialised           bool
}

func (a *ChainerAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Tools == nil {
		a.Tools = capability.NewRegistry()
	}
	// Phase 2: Initialize recovery manager if checkpoint store configured
	if a.CheckpointStore != nil && a.RecoveryManager == nil {
		a.RecoveryManager = checkpoint.NewRecoveryManager(a.CheckpointStore)
	}
	a.initialised = true
	return nil
}

func (a *ChainerAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityExecute,
		core.CapabilityExplain,
	}
}

func (a *ChainerAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	chain, err := a.resolveChain(task)
	if err != nil {
		return nil, err
	}
	g := graph.NewGraph()
	nodes := make([]graph.Node, 0, len(chain.Links)+1)
	for i, link := range chain.Links {
		nodes = append(nodes, &chainerLinkNode{id: fmt.Sprintf("chainer_link_%02d_%s", i, sanitizeLinkName(link.Name)), name: link.Name})
	}
	nodes = append(nodes, graph.NewTerminalNode("chainer_done"))
	for _, node := range nodes {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	if err := g.SetStart(nodes[0].ID()); err != nil {
		return nil, err
	}
	for i := 0; i < len(nodes)-1; i++ {
		if err := g.AddEdge(nodes[i].ID(), nodes[i+1].ID(), nil, false); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func (a *ChainerAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if !a.initialised {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	if state == nil {
		state = core.NewContext()
	}

	chain, err := a.resolveChain(task)
	if err != nil {
		return nil, err
	}
	if err := chain.Validate(); err != nil {
		return nil, err
	}

	// Phase 2: Choose execution mode
	if a.CheckpointStore != nil {
		// Use pipeline-based execution with checkpointing
		return a.executePipeline(ctx, task, state, chain)
	}

	// Legacy execution (Phase 1)
	return a.executeLegacy(ctx, task, state, chain)
}

// executeLegacy runs the chain using the custom chainRunner (Phase 1 behavior).
func (a *ChainerAgent) executeLegacy(ctx context.Context, task *core.Task, state *core.Context, chain *Chain) (*core.Result, error) {
	if err := (&chainRunner{Model: a.Model}).Run(ctx, task, chain, state); err != nil {
		return nil, err
	}
	state.Set("chainer.links_executed", len(chain.Links))
	data := map[string]any{"links_executed": len(chain.Links)}
	for _, link := range chain.Links {
		if value, ok := state.Get(link.OutputKey); ok {
			data[link.OutputKey] = value
		}
	}
	return &core.Result{Success: true, Data: data}, nil
}

// executePipeline runs the chain using framework/pipeline.Runner with checkpointing (Phase 2).
// Optionally tracks token budget and applies compression (Phase 3).
func (a *ChainerAgent) executePipeline(ctx context.Context, task *core.Task, state *core.Context, chain *Chain) (*core.Result, error) {
	// Build pipeline stages from links.
	// LinkStage is in the chainer package (stage_adapter.go), so no import cycle
	pipelineStages := make([]pipeline.Stage, 0, len(chain.Links))
	for _, link := range chain.Links {
		linkCopy := link // avoid loop variable capture
		stage := NewLinkStage(&linkCopy, a.Model)
		pipelineStages = append(pipelineStages, stage)
	}

	// Store task instruction in context for stage prompts
	state.Set("__chainer_instruction", task.Instruction)

	// Phase 3: Wire up budget tracking if configured
	if a.BudgetManager != nil && a.CompressionListener != nil {
		a.BudgetManager.AddListener(a.CompressionListener)
		state.Set("__budget_manager", a.BudgetManager)
	}

	// Try to find checkpoint for resumption
	var resumeCP *pipeline.Checkpoint
	if a.RecoveryManager != nil && task.ID != "" {
		resumeCP, _ = a.RecoveryManager.FindLastCheckpoint(task.ID)
		// Phase 4: Record resume event if resuming from checkpoint
		if resumeCP != nil && a.EventRecorder != nil {
			event := telemetry.ResumeEvent(task.ID, resumeCP.StageIndex)
			_ = a.EventRecorder.Record(event)
		}
	}

	// Phase 4: Wire up event recorder if configured
	if a.EventRecorder != nil {
		state.Set("__event_recorder", a.EventRecorder)
	}

	// Execute pipeline with checkpointing
	runner := &pipeline.Runner{
		Options: pipeline.RunnerOptions{
			Model:                a.Model,
			CheckpointStore:      a.CheckpointStore,
			CheckpointAfterStage: a.CheckpointAfterStage,
			ResumeCheckpoint:     resumeCP,
		},
	}

	results, err := runner.Execute(ctx, task, state, pipelineStages)
	if err != nil {
		return nil, err
	}

	// Extract results
	state.Set("chainer.links_executed", len(results))
	data := map[string]any{
		"links_executed": len(results),
		"stage_results":  results,
	}

	// Phase 3: Include budget metrics if tracking was enabled
	if a.BudgetManager != nil {
		data["budget_metrics"] = a.BudgetManager.Budget()
	}

	// Collect outputs from each result
	for _, link := range chain.Links {
		if value, ok := state.Get(link.OutputKey); ok {
			data[link.OutputKey] = value
		}
	}

	// Clear checkpoints on success (optional cleanup)
	if a.RecoveryManager != nil && task.ID != "" {
		_ = a.RecoveryManager.ClearCheckpoints(task.ID)
	}

	return &core.Result{Success: true, Data: data}, nil
}

// Phase 4: Telemetry Query Methods

// ExecutionEvents returns all recorded events for a task.
func (a *ChainerAgent) ExecutionEvents(taskID string) []*telemetry.ChainerEvent {
	if a == nil || a.EventRecorder == nil {
		return nil
	}
	return a.EventRecorder.AllEvents(taskID)
}

// ExecutionSummary returns a high-level overview of task execution.
func (a *ChainerAgent) ExecutionSummary(taskID string) *telemetry.ExecutionSummary {
	if a == nil || a.EventRecorder == nil {
		return nil
	}
	return a.EventRecorder.Summary(taskID)
}

// LinkEvents returns events for a specific link in a task.
func (a *ChainerAgent) LinkEvents(taskID, linkName string) []*telemetry.ChainerEvent {
	if a == nil || a.EventRecorder == nil {
		return nil
	}
	return a.EventRecorder.RecordedEvents(taskID, linkName)
}

func (a *ChainerAgent) resolveChain(task *core.Task) (*Chain, error) {
	switch {
	case a.ChainBuilder != nil:
		return a.ChainBuilder(task)
	case a.Chain != nil:
		return a.Chain, nil
	default:
		return nil, fmt.Errorf("chainer: chain not configured")
	}
}


type chainerLinkNode struct {
	id   string
	name string
}

func (n *chainerLinkNode) ID() string                { return n.id }
func (n *chainerLinkNode) Type() graph.NodeType      { return graph.NodeTypeSystem }
func (n *chainerLinkNode) Execute(_ context.Context, state *core.Context) (*core.Result, error) {
	if state != nil {
		state.Set("chainer.inspect_link", n.name)
	}
	return &core.Result{NodeID: n.id, Success: true}, nil
}

func sanitizeLinkName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	if name == "" {
		return "link"
	}
	return name
}
