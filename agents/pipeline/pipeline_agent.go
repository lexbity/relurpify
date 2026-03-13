package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/agents/internal/workflowutil"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	frameworkpipeline "github.com/lexcodex/relurpify/framework/pipeline"
)

// PipelineStageFactory resolves pipeline stages for a task.
type PipelineStageFactory interface {
	StagesForTask(task *core.Task) ([]frameworkpipeline.Stage, error)
}

// PipelineAgent executes a deterministic sequence of typed pipeline stages.
type PipelineAgent struct {
	Model              core.LanguageModel
	Config             *core.Config
	Tools              *capability.Registry
	WorkflowStatePath  string
	ResumeCheckpointID string

	Stages       []frameworkpipeline.Stage
	StageBuilder func(task *core.Task) ([]frameworkpipeline.Stage, error)
	StageFactory PipelineStageFactory
}

func (a *PipelineAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	return nil
}

func (a *PipelineAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if task == nil {
		return nil, fmt.Errorf("task required")
	}
	if a.Model == nil {
		return nil, fmt.Errorf("pipeline agent missing language model")
	}
	if state == nil {
		state = core.NewContext()
	}
	stages, err := a.stagesForTask(task)
	if err != nil {
		return nil, err
	}
	if len(stages) == 0 {
		return nil, fmt.Errorf("pipeline agent has no stages for task")
	}

	var store *db.SQLiteWorkflowStateStore
	var workflowID, runID string
	executionTask := task
	if strings.TrimSpace(a.WorkflowStatePath) != "" {
		store, workflowID, runID, err = a.openWorkflowStore(ctx, task, state)
		if err != nil {
			return nil, err
		}
		if store != nil {
			if retrievalPayload, retrievalErr := workflowutil.Hydrate(ctx, store, workflowID, workflowutil.RetrievalQuery{
				Primary:  task.Instruction,
				TaskText: task.Instruction,
			}, 4, 500); retrievalErr != nil {
				_ = store.Close()
				return nil, retrievalErr
			} else if len(retrievalPayload) > 0 {
				workflowutil.ApplyState(state, "pipeline.workflow_retrieval", retrievalPayload)
				executionTask = workflowutil.ApplyTask(task, retrievalPayload)
			}
			defer store.Close()
		}
	}

	runner := &frameworkpipeline.Runner{Options: frameworkpipeline.RunnerOptions{
		Model:             a.Model,
		ModelName:         a.modelName(),
		Tools:             a.availableTools(),
		EnableToolCalling: a.toolCallingEnabled(),
		Telemetry:         a.telemetry(),
		CapabilityInvoker: a.Tools,
	}}
	if store != nil {
		runner.Options.CheckpointStore = NewSQLitePipelineCheckpointStore(store, workflowID, runID)
		runner.Options.CheckpointAfterStage = true
	}
	if strings.TrimSpace(a.ResumeCheckpointID) != "" && store != nil {
		cp, err := NewSQLitePipelineCheckpointStore(store, workflowID, runID).Load(task.ID, a.ResumeCheckpointID)
		if err != nil {
			return nil, fmt.Errorf("pipeline resume: %w", err)
		}
		runner.Options.ResumeCheckpoint = cp
	}
	results, err := runner.Execute(ctx, executionTask, state, stages)
	if store != nil {
		if persistErr := a.persistStageResults(ctx, store, workflowID, runID, results); persistErr != nil && err == nil {
			err = persistErr
		}
	}
	if err != nil {
		return nil, err
	}

	final := map[string]any{
		"workflow_id": workflowID,
		"run_id":      runID,
		"stages":      len(results),
	}
	if len(results) > 0 {
		last := results[len(results)-1]
		final["stage_name"] = last.StageName
		final["decoded_output"] = last.DecodedOutput
	}
	state.Set("pipeline.results", results)
	state.Set("pipeline.final_output", final)
	return &core.Result{
		Success: true,
		Data: map[string]any{
			"stage_results": results,
			"final_output":  final,
		},
	}, nil
}

func (a *PipelineAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityExecute,
		core.CapabilityCode,
		core.CapabilityExplain,
	}
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
	nodes := make([]graph.Node, 0, len(stages)+1)
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
	if err := g.SetStart(nodes[0].ID()); err != nil {
		return nil, err
	}
	for idx := 0; idx < len(nodes)-1; idx++ {
		if err := g.AddEdge(nodes[idx].ID(), nodes[idx+1].ID(), nil, false); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func (a *PipelineAgent) stagesForTask(task *core.Task) ([]frameworkpipeline.Stage, error) {
	switch {
	case a.StageBuilder != nil:
		return a.StageBuilder(task)
	case a.StageFactory != nil:
		return a.StageFactory.StagesForTask(task)
	case len(a.Stages) > 0:
		return append([]frameworkpipeline.Stage{}, a.Stages...), nil
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
	if a == nil || a.Tools == nil {
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
	return a.Config.OllamaToolCalling
}

func (a *PipelineAgent) openWorkflowStore(ctx context.Context, task *core.Task, state *core.Context) (*db.SQLiteWorkflowStateStore, string, string, error) {
	store, err := db.NewSQLiteWorkflowStateStore(filepath.Clean(a.WorkflowStatePath))
	if err != nil {
		return nil, "", "", err
	}
	workflowID := strings.TrimSpace(state.GetString("pipeline.workflow_id"))
	if workflowID == "" {
		if task != nil && task.Context != nil {
			if raw, ok := task.Context["workflow_id"]; ok {
				workflowID = strings.TrimSpace(fmt.Sprint(raw))
			}
		}
	}
	if workflowID == "" {
		workflowID = fmt.Sprintf("pipeline-%s", fallbackTaskID(task))
	}
	runID := strings.TrimSpace(state.GetString("pipeline.run_id"))
	if runID == "" {
		runID = fmt.Sprintf("%s-run-%d", fallbackTaskID(task), time.Now().UnixNano())
	}
	if _, ok, err := store.GetWorkflow(ctx, workflowID); err != nil {
		_ = store.Close()
		return nil, "", "", err
	} else if !ok {
		if err := store.CreateWorkflow(ctx, memory.WorkflowRecord{
			WorkflowID:  workflowID,
			TaskID:      fallbackTaskID(task),
			TaskType:    taskType(task),
			Instruction: taskInstruction(task),
			Status:      memory.WorkflowRunStatusRunning,
			Metadata:    map[string]any{"agent": "pipeline"},
		}); err != nil {
			_ = store.Close()
			return nil, "", "", err
		}
	}
	if _, ok, err := store.GetRun(ctx, runID); err != nil {
		_ = store.Close()
		return nil, "", "", err
	} else if !ok {
		if err := store.CreateRun(ctx, memory.WorkflowRunRecord{
			RunID:      runID,
			WorkflowID: workflowID,
			Status:     memory.WorkflowRunStatusRunning,
			AgentName:  "pipeline",
			StartedAt:  time.Now().UTC(),
		}); err != nil {
			_ = store.Close()
			return nil, "", "", err
		}
	}
	state.Set("pipeline.workflow_id", workflowID)
	state.Set("pipeline.run_id", runID)
	return store, workflowID, runID, nil
}

func (a *PipelineAgent) persistStageResults(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID, runID string, results []frameworkpipeline.StageResult) error {
	for idx, result := range results {
		responseJSON := ""
		if result.Response != nil {
			data, err := json.Marshal(result.Response)
			if err != nil {
				return err
			}
			responseJSON = string(data)
		}
		record := memory.WorkflowStageResultRecord{
			ResultID:         fmt.Sprintf("%s-stage-%02d-attempt-%02d", runID, idx+1, result.RetryAttempt),
			WorkflowID:       workflowID,
			RunID:            runID,
			StageName:        result.StageName,
			StageIndex:       idx,
			ContractName:     result.ContractName,
			ContractVersion:  result.ContractVersion,
			PromptText:       result.Prompt,
			ResponseJSON:     responseJSON,
			DecodedOutput:    result.DecodedOutput,
			ValidationOK:     result.ValidationOK,
			ErrorText:        result.ErrorText,
			RetryAttempt:     result.RetryAttempt,
			TransitionKind:   string(result.Transition.Kind),
			NextStage:        result.Transition.NextStage,
			TransitionReason: result.Transition.Reason,
			StartedAt:        result.StartedAt,
			FinishedAt:       result.FinishedAt,
		}
		if err := store.SaveStageResult(ctx, record); err != nil {
			return err
		}
	}
	return store.UpdateRunStatus(ctx, runID, memory.WorkflowRunStatusCompleted, nil)
}

type pipelineStageNode struct {
	id    string
	stage frameworkpipeline.Stage
}

// pipelineStageNode is a visualization-only stub used by BuildGraph().
func (n *pipelineStageNode) ID() string { return n.id }

func (n *pipelineStageNode) Type() graph.NodeType { return graph.NodeTypeSystem }

func (n *pipelineStageNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	if n.stage != nil && state != nil {
		state.Set("pipeline.inspect_stage", n.stage.Name())
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
	return task.Type
}

func taskInstruction(task *core.Task) string {
	if task == nil {
		return ""
	}
	return task.Instruction
}
