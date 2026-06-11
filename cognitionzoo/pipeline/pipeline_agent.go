package pipeline

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/ports"
	capability "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	execution "codeburg.org/lexbit/relurpify/execution"
	graph "codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/model"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// PipelineStageFactory resolves pipeline stages for a task.
type PipelineStageFactory interface {
	StagesForTask(task *execution.Task) ([]Stage, error)
}

// PipelineAgent executes a deterministic sequence of typed pipeline stages.
type PipelineAgent struct {
	Model             model.LanguageModel
	Config            *execution.Config
	Tools             *capability.CapabilityRegistry
	WorkflowStatePath string

	Stages       []Stage
	StageBuilder func(task *execution.Task) ([]Stage, error)
	StageFactory PipelineStageFactory

	StreamMode      contextstream.Mode
	StreamQuery     string
	StreamMaxTokens int

	executionCatalog *capability.ExecutionCapabilityCatalogSnapshot
}

func (a *PipelineAgent) Initialize(cfg *execution.Config) error {
	a.Config = cfg
	return nil
}

func (a *PipelineAgent) Execute(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*execution.Result, error) {
	a.executionCatalog = nil
	if a.Tools != nil {
		a.executionCatalog = a.Tools.CaptureExecutionCatalogSnapshot(ctx)
	}
	defer func() {
		a.executionCatalog = nil
	}()
	if task == nil {
		return nil, fmt.Errorf("task required")
	}
	if a.Model == nil {
		return nil, fmt.Errorf("pipeline agent missing language model")
	}
	if env == nil {
		env = contextdata.NewEnvelope("pipeline", "session")
	}
	stages, err := a.stagesForTask(task)
	if err != nil {
		return nil, err
	}
	if len(stages) == 0 {
		return nil, fmt.Errorf("pipeline agent has no stages for task")
	}

	executionTask := task
	// Workflow store functionality temporarily disabled - memory/db package being rebuilt
	_ = a.WorkflowStatePath

	runner := &Runner{Options: RunnerOptions{
		Model:             a.Model,
		ModelName:         a.modelName(),
		Tools:             a.availableTools(ctx),
		EnableToolCalling: a.toolCallingEnabled(),
		AgentSpec:         a.Config.AgentSpec,
		Telemetry:         a.telemetry(),
		CapabilityInvoker: a.Tools,
	}}
	results, err := runner.Execute(ctx, executionTask, env, stages)
	if err != nil {
		return nil, err
	}

	final := map[string]any{
		"workflow_id": "",
		"run_id":      "",
		"stages":      len(results),
	}
	if len(results) > 0 {
		last := results[len(results)-1]
		final["stage_name"] = last.StageName
		final["decoded_output"] = last.DecodedOutput
	}
	if _, ok := contextdata.GetTyped[any](env, "pipeline.results"); !ok {
		env.SetWorkingValueWithClass("pipeline.results", results, contextdata.MemoryClassTask)
	}
	if _, ok := contextdata.GetTyped[any](env, "pipeline.final_output"); !ok {
		env.SetWorkingValueWithClass("pipeline.final_output", final, contextdata.MemoryClassTask)
	}
	env.SetWorkingValueWithClass("pipeline.results_summary", summarizePipelineResults(results), contextdata.MemoryClassTask)
	return &execution.Result{
		Success: true,
		Data: execution.NewToolResultPayload(map[string]any{
			"stage_results": results,
			"final_output":  final,
		}),
	}, nil
}


func (a *PipelineAgent) Capabilities() []string {
	return []string{"pipeline"}
}

// BuildGraph returns a visualization graph of the pipeline stage sequence.
// The returned graph is not executable; stage nodes are stubs that record
// inspection metadata but do not invoke stage logic. Use Execute for actual
// pipeline execution. A fully executable graph integration is planned for Phase 8.
func (a *PipelineAgent) BuildGraph(ctx context.Context, task *execution.Task) (*graph.Graph, error) {
	stages, err := a.stagesForTask(task)
	if err != nil {
		return nil, err
	}
	if len(stages) == 0 {
		return nil, fmt.Errorf("pipeline agent has no stages for task")
	}
	g := graph.NewGraph()
	stream := a.streamTriggerNode(task)
	nodes := make([]graph.Node, 0, len(stages)+2)
	if stream != nil {
		nodes = append(nodes, stream)
	}
	for idx, stage := range stages {
		nodes = append(nodes, &pipelineStageNode{
			id:    fmt.Sprintf("pipeline_stage_%02d_%s", idx+1, sanitizePipelineName(stage.Name())),
			stage: stage,
		})
	}
	done := graph.NewTerminalNode("pipeline_done")
	nodes = append(nodes, done)
	for _, node := range nodes {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	// Set start to stream node if present, otherwise first stage
	var startIdx int
	if stream != nil {
		startIdx = 0
	} else {
		startIdx = 1
	}
	if err := g.SetStart(nodes[startIdx].ID()); err != nil {
		return nil, err
	}
	// Connect nodes in sequence
	for idx := 0; idx < len(nodes)-1; idx++ {
		if err := g.AddEdge(nodes[idx].ID(), nodes[idx+1].ID(), nil, false); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func (a *PipelineAgent) stagesForTask(task *execution.Task) ([]Stage, error) {
	switch {
	case a.StageBuilder != nil:
		return a.StageBuilder(task)
	case a.StageFactory != nil:
		return a.StageFactory.StagesForTask(task)
	case len(a.Stages) > 0:
		return append([]Stage{}, a.Stages...), nil
	default:
		return nil, errors.New("pipeline stages not configured")
	}
}

func (a *PipelineAgent) telemetry() telemetry.Telemetry {
	if a == nil || a.Config == nil {
		return nil
	}
	return a.Config.Telemetry
}

func (a *PipelineAgent) availableTools(ctx context.Context) []ports.Tool {
	if a == nil {
		return nil
	}
	if a.executionCatalog != nil {
		return a.executionCatalog.ModelCallableTools(ctx)
	}
	if a.Tools == nil {
		return nil
	}
	return a.Tools.ModelCallableTools(ctx)
}

func (a *PipelineAgent) modelName() string {
	if a == nil || a.Config == nil {
		return ""
	}
	return a.Config.Model
}

func (a *PipelineAgent) toolCallingEnabled() bool {
	if a == nil || a.Config == nil {
		return false
	}
	return a.Config.NativeToolCalling
}


func summarizePipelineResults(results []StageResult) string {
	if len(results) == 0 {
		return ""
	}
	parts := make([]string, 0, len(results))
	for _, result := range results {
		status := "ok"
		if !result.ValidationOK {
			status = "invalid"
		}
		if strings.TrimSpace(result.ErrorText) != "" {
			status = "error"
		}
		parts = append(parts, fmt.Sprintf("%s [%s]", result.StageName, status))
	}
	return strings.Join(parts, "; ")
}

type pipelineStageNode struct {
	id    string
	stage Stage
}

// pipelineStageNode is a visualization-only stub used by BuildGraph().
func (n *pipelineStageNode) ID() string { return n.id }

func (n *pipelineStageNode) Type() graph.NodeType { return graph.NodeTypeSystem }

func (n *pipelineStageNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if n.stage != nil && env != nil {
		env.SetWorkingValueWithClass("pipeline.inspect_stage", n.stage.Name(), contextdata.MemoryClassTask)
	}
	return &execution.Result{NodeID: n.id, Success: true}, nil
}

func sanitizePipelineName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	if name == "" {
		return "stage"
	}
	return name
}

// streamMode returns the streaming mode, defaulting to blocking.
func (a *PipelineAgent) streamMode() contextstream.Mode {
	if a.StreamMode != "" {
		return a.StreamMode
	}
	return contextstream.ModeBlocking
}

// streamQuery returns the query for streaming, defaulting to task instruction.
func (a *PipelineAgent) streamQuery(task *execution.Task) string {
	if a.StreamQuery != "" {
		return a.StreamQuery
	}
	if task != nil {
		return task.Instruction
	}
	return ""
}

// streamMaxTokens returns the max tokens for streaming, defaulting to 256.
func (a *PipelineAgent) streamMaxTokens() int {
	if a.StreamMaxTokens > 0 {
		return a.StreamMaxTokens
	}
	return 256
}

// streamTriggerNode creates a streaming trigger node for the pipeline agent.
func (a *PipelineAgent) streamTriggerNode(task *execution.Task) graph.Node {
	query := a.streamQuery(task)
	node := graph.NewContextStreamNode("pipeline_stream", retrieval.RetrievalQuery{Text: query}, a.streamMaxTokens())
	node.Mode = a.streamMode()
	return node
}
