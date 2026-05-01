package pipeline

import (
	"context"
	"errors"
	"fmt"
	"strings"

	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

// PipelineStageFactory resolves pipeline stages for a task.
type PipelineStageFactory interface {
	StagesForTask(task *core.Task) ([]Stage, error)
}

// PipelineAgent executes a deterministic sequence of typed pipeline stages.
type PipelineAgent struct {
	Model             core.LanguageModel
	Config            *core.Config
	Tools             *capability.Registry
	WorkflowStatePath string

	Stages       []Stage
	StageBuilder func(task *core.Task) ([]Stage, error)
	StageFactory PipelineStageFactory

	StreamMode      contextstream.Mode
	StreamQuery     string
	StreamMaxTokens int

	executionCatalog *capability.ExecutionCapabilityCatalogSnapshot
}

func (a *PipelineAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	return nil
}

func (a *PipelineAgent) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error) {
	a.executionCatalog = nil
	if a.Tools != nil {
		a.executionCatalog = a.Tools.CaptureExecutionCatalogSnapshot()
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
		Tools:             a.availableTools(),
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
	if _, ok := env.GetWorkingValue("pipeline.results"); !ok {
		env.SetWorkingValue("pipeline.results", results, contextdata.MemoryClassTask)
	}
	if _, ok := env.GetWorkingValue("pipeline.final_output"); !ok {
		env.SetWorkingValue("pipeline.final_output", final, contextdata.MemoryClassTask)
	}
	env.SetWorkingValue("pipeline.results_summary", summarizePipelineResults(results), contextdata.MemoryClassTask)
	return &core.Result{
		Success: true,
		Data: map[string]any{
			"stage_results": results,
			"final_output":  final,
		},
	}, nil
}

func compactPipelineResultsState(results []StageResult) map[string]any {
	value := map[string]any{
		"stage_count": len(results),
	}
	if len(results) == 0 {
		return value
	}
	stages := make([]map[string]any, 0, len(results))
	for _, result := range results {
		stages = append(stages, map[string]any{
			"name":          result.StageName,
			"validation_ok": result.ValidationOK,
			"error_text":    result.ErrorText,
			"transition":    result.Transition.Kind,
		})
	}
	value["stages"] = stages
	last := results[len(results)-1]
	value["last_stage"] = map[string]any{
		"name":          last.StageName,
		"validation_ok": last.ValidationOK,
		"error_text":    last.ErrorText,
		"transition":    last.Transition.Kind,
	}
	return value
}

func compactPipelineFinalOutputState(final map[string]any, results []StageResult) map[string]any {
	value := map[string]any{
		"stages":  len(results),
		"summary": summarizePipelineResults(results),
	}
	if stageName := strings.TrimSpace(fmt.Sprint(final["stage_name"])); stageName != "" && stageName != "<nil>" {
		value["stage_name"] = stageName
	}
	if workflowID := strings.TrimSpace(fmt.Sprint(final["workflow_id"])); workflowID != "" && workflowID != "<nil>" {
		value["workflow_id"] = workflowID
	}
	if runID := strings.TrimSpace(fmt.Sprint(final["run_id"])); runID != "" && runID != "<nil>" {
		value["run_id"] = runID
	}
	return value
}

func (a *PipelineAgent) Capabilities() []string {
	return []string{"pipeline"}
}

// BuildGraph returns a visualization graph of the pipeline stage sequence.
// The returned graph is not executable; stage nodes are stubs that record
// inspection metadata but do not invoke stage logic. Use Execute for actual
// pipeline execution. A fully executable graph integration is planned for Phase 8.
func (a *PipelineAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
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
	startIdx := 0
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

func (a *PipelineAgent) stagesForTask(task *core.Task) ([]Stage, error) {
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

func (a *PipelineAgent) telemetry() core.Telemetry {
	if a == nil || a.Config == nil {
		return nil
	}
	return a.Config.Telemetry
}

func (a *PipelineAgent) availableTools() []core.Tool {
	if a == nil {
		return nil
	}
	if a.executionCatalog != nil {
		return a.executionCatalog.ModelCallableTools()
	}
	if a.Tools == nil {
		return nil
	}
	return a.Tools.ModelCallableTools()
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

func (a *PipelineAgent) openWorkflowStore(ctx context.Context, task *core.Task, env *contextdata.Envelope) (any, string, string, error) {
	_ = ctx
	_ = task
	_ = env
	return nil, "", "", fmt.Errorf("workflow store temporarily unavailable")
}

func (a *PipelineAgent) persistStageResults(ctx context.Context, store any, workflowID, runID string, results []StageResult) error {
	_ = ctx
	_ = store
	_ = workflowID
	_ = runID
	_ = results
	return nil
}

func (a *PipelineAgent) persistResultsArtifact(ctx context.Context, store any, workflowID, runID string, results []StageResult) (*core.ArtifactReference, error) {
	_ = ctx
	_ = store
	_ = workflowID
	_ = runID
	_ = results
	return nil, nil
}

func (a *PipelineAgent) persistFinalOutputArtifact(ctx context.Context, store any, workflowID, runID string, final map[string]any) (*core.ArtifactReference, error) {
	_ = ctx
	_ = store
	_ = workflowID
	_ = runID
	_ = final
	return nil, nil
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

func summarizePipelineFinalOutput(final map[string]any) string {
	if len(final) == 0 {
		return ""
	}
	stage := strings.TrimSpace(fmt.Sprint(final["stage_name"]))
	if stage == "" || stage == "<nil>" {
		stage = "pipeline"
	}
	return fmt.Sprintf("%s final output", stage)
}

type pipelineStageNode struct {
	id    string
	stage Stage
}

// pipelineStageNode is a visualization-only stub used by BuildGraph().
func (n *pipelineStageNode) ID() string { return n.id }

func (n *pipelineStageNode) Type() graph.NodeType { return graph.NodeTypeSystem }

func (n *pipelineStageNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	if n.stage != nil && env != nil {
		env.SetWorkingValue("pipeline.inspect_stage", n.stage.Name(), contextdata.MemoryClassTask)
	}
	return &core.Result{NodeID: n.id, Success: true}, nil
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

func fallbackTaskID(task *core.Task) string {
	if task != nil && strings.TrimSpace(task.ID) != "" {
		return strings.TrimSpace(task.ID)
	}
	return "task"
}

func taskType(task *core.Task) core.TaskType {
	if task == nil || task.Type == "" {
		return core.TaskTypeCodeGeneration
	}
	return core.TaskType(task.Type)
}

func taskInstruction(task *core.Task) string {
	if task == nil {
		return ""
	}
	return task.Instruction
}

// streamMode returns the streaming mode, defaulting to blocking.
func (a *PipelineAgent) streamMode() contextstream.Mode {
	if a.StreamMode != "" {
		return a.StreamMode
	}
	return contextstream.ModeBlocking
}

// streamQuery returns the query for streaming, defaulting to task instruction.
func (a *PipelineAgent) streamQuery(task *core.Task) string {
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
func (a *PipelineAgent) streamTriggerNode(task *core.Task) graph.Node {
	query := a.streamQuery(task)
	node := graph.NewContextStreamNode("pipeline_stream", retrieval.RetrievalQuery{Text: query}, a.streamMaxTokens())
	node.Mode = a.streamMode()
	return node
}
