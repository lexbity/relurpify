package blackboard

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
)

// BlackboardAgent implements graph.WorkflowExecutor using the Blackboard architecture.
// A shared Blackboard workspace is maintained across a control loop; multiple
// KnowledgeSource specialists read and write it each cycle. Execution order is
// data-driven rather than structurally predetermined.
type BlackboardAgent struct {
	// Model is the language model available to knowledge sources.
	Model core.LanguageModel
	// Tools is the capability registry available to knowledge sources.
	Tools *capability.Registry
	// Memory is the memory store for the agent.
	Memory memory.MemoryStore
	// Config holds runtime configuration.
	Config *core.Config
	// Sources is the set of knowledge sources evaluated each cycle.
	// When empty, DefaultKnowledgeSources() is used.
	Sources []KnowledgeSource
	// CheckpointPath is an optional filesystem path for checkpoint storage.
	CheckpointPath string
	// MaxCycles is the upper bound on control-loop iterations (default 20).
	MaxCycles int

	initialised bool
}

// Initialize satisfies graph.WorkflowExecutor. It wires configuration and ensures
// knowledge sources are populated.
func (a *BlackboardAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Tools == nil {
		a.Tools = capability.NewRegistry()
	}
	if len(a.Sources) == 0 {
		a.Sources = DefaultKnowledgeSources()
	}
	a.initialised = true
	return nil
}

// Capabilities declares what this agent can do.
func (a *BlackboardAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityExecute,
		core.CapabilityCode,
		core.CapabilityReview,
	}
}

// BuildGraph returns a minimal single-node graph suitable for agenttest and
// visualisation. Blackboard execution is driven by Execute, not a static graph.
func (a *BlackboardAgent) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("bb_done")
	if err := g.AddNode(done); err != nil {
		return nil, err
	}
	if err := g.SetStart("bb_done"); err != nil {
		return nil, err
	}
	return g, nil
}

// Execute initialises the blackboard with the task goal and runs the controller
// loop until the goal is satisfied or an error occurs.
func (a *BlackboardAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if !a.initialised {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	if state == nil {
		state = core.NewContext()
	}

	goal := ""
	if task != nil {
		goal = task.Instruction
	}

	bb := NewBlackboard(goal)

	// Restore blackboard from context if a previous partial run placed it there.
	if raw, ok := state.Get("blackboard"); ok {
		if restored, ok := raw.(*Blackboard); ok {
			bb = restored
		}
	}

	controller := &Controller{
		Sources:   a.Sources,
		MaxCycles: a.MaxCycles,
	}

	if err := controller.Run(ctx, bb, a.Tools, a.Model); err != nil {
		// Persist partial blackboard so a resumed run can continue.
		state.Set("blackboard", bb)
		return nil, fmt.Errorf("blackboard: controller failed: %w", err)
	}

	// Persist final blackboard into state for downstream consumers.
	state.Set("blackboard", bb)
	state.Set("blackboard.artifact_count", len(bb.Artifacts))

	// Collect artifact contents for the result payload.
	artifactSummaries := make([]string, 0, len(bb.Artifacts))
	for _, art := range bb.Artifacts {
		artifactSummaries = append(artifactSummaries, fmt.Sprintf("[%s] %s: %s", art.Kind, art.ID, art.Content))
	}

	return &core.Result{
		Success: true,
		Data: map[string]any{
			"artifacts":       artifactSummaries,
			"artifact_count":  len(bb.Artifacts),
			"fact_count":      len(bb.Facts),
			"issue_count":     len(bb.Issues),
			"completed_count": len(bb.CompletedActions),
		},
	}, nil
}
