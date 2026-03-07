package pipeline

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/toolsys"
)

// RunnerOptions configures how a pipeline executes stages.
type RunnerOptions struct {
	Model                core.LanguageModel
	ModelName            string
	Tools                []core.Tool
	EnableToolCalling    bool
	Telemetry            core.Telemetry
	CheckpointStore      CheckpointStore
	CheckpointAfterStage bool
	ResumeCheckpoint     *Checkpoint
}

// Runner executes a linear sequence of typed stages.
type Runner struct {
	Options RunnerOptions
}

// Execute runs the provided stages in order, optionally resuming from a checkpoint.
func (r *Runner) Execute(ctx context.Context, task *core.Task, state *core.Context, stages []Stage) ([]StageResult, error) {
	if len(stages) == 0 {
		return nil, errors.New("pipeline stages required")
	}
	if r.Options.Model == nil {
		return nil, errors.New("pipeline runner model required")
	}
	if state == nil {
		state = core.NewContext()
	}
	taskID := ""
	if task != nil {
		taskID = task.ID
	}
	startIndex, state, priorResults, err := r.resume(taskID, state)
	if err != nil {
		return nil, err
	}

	results := append([]StageResult{}, priorResults...)
	for idx := startIndex; idx < len(stages); idx++ {
		stage := stages[idx]
		if err := ValidateStage(stage); err != nil {
			return results, err
		}

		stageResult, err := r.executeStage(ctx, task, taskID, state, stage, idx)
		results = append(results, stageResult)
		if r.Options.CheckpointStore != nil && r.Options.CheckpointAfterStage {
			cp := &Checkpoint{
				CheckpointID: fmt.Sprintf("pipeline_ckpt_%d", time.Now().UnixNano()),
				TaskID:       taskID,
				StageName:    stage.Name(),
				StageIndex:   idx,
				CreatedAt:    time.Now().UTC(),
				Context:      state.Clone(),
				Result:       stageResult,
			}
			if err := validateCheckpoint(cp); err != nil {
				return results, err
			}
			if err := r.Options.CheckpointStore.Save(cp); err != nil {
				return results, err
			}
		}
		if err != nil {
			return results, err
		}
		if stageResult.Transition.Kind == TransitionStop {
			break
		}
	}
	return results, nil
}

func (r *Runner) resume(taskID string, state *core.Context) (int, *core.Context, []StageResult, error) {
	cp := r.Options.ResumeCheckpoint
	if cp == nil {
		return 0, state, nil, nil
	}
	if err := validateCheckpoint(cp); err != nil {
		return 0, state, nil, err
	}
	if taskID != "" && cp.TaskID != "" && cp.TaskID != taskID {
		return 0, state, nil, fmt.Errorf("pipeline checkpoint task mismatch: checkpoint=%s task=%s", cp.TaskID, taskID)
	}
	return cp.StageIndex + 1, cp.Context.Clone(), []StageResult{cp.Result}, nil
}

func (r *Runner) executeStage(ctx context.Context, task *core.Task, taskID string, state *core.Context, stage Stage, index int) (StageResult, error) {
	contract := stage.Contract()
	result := StageResult{
		StageName:       stage.Name(),
		ContractName:    contract.Name,
		ContractVersion: contract.Metadata.SchemaVersion,
		StartedAt:       time.Now().UTC(),
		Transition: StageTransition{
			Kind: TransitionNext,
		},
	}
	emitStageEvent(r.Options.Telemetry, pipelineEventStageStart, taskID, stage.Name(), "", map[string]any{
		"stage_index":      index,
		"contract_name":    contract.Name,
		"contract_version": contract.Metadata.SchemaVersion,
	})

	maxRetries := contract.Metadata.RetryPolicy.MaxAttempts
	stageTools := resolveStageTools(ctx, state, stage, r.Options.Tools)
	for attempt := 0; attempt <= maxRetries; attempt++ {
		result.RetryAttempt = attempt

		prompt, err := stage.BuildPrompt(state)
		if err != nil {
			result.ErrorText = err.Error()
			result.FinishedAt = time.Now().UTC()
			return result, err
		}
		result.Prompt = prompt

		resp, usedTools, err := r.generateStageResponse(ctx, task, state, stage, prompt, stageTools)
		if err != nil {
			result.ErrorText = err.Error()
			result.FinishedAt = time.Now().UTC()
			return result, err
		}
		result.Response = resp
		if requiresToolExecution(stage, task, state, stageTools) && !usedTools {
			err := fmt.Errorf("pipeline stage %s requires a tool call before returning output", stage.Name())
			result.ErrorText = err.Error()
			result.FinishedAt = time.Now().UTC()
			emitStageEvent(r.Options.Telemetry, pipelineEventStageValidError, taskID, stage.Name(), err.Error(), map[string]any{
				"stage_index":   index,
				"retry_attempt": attempt,
			})
			if attempt < maxRetries {
				result.Transition = StageTransition{Kind: TransitionRetry, Reason: err.Error()}
				continue
			}
			result.Transition = StageTransition{Kind: TransitionStop, Reason: err.Error()}
			return result, err
		}

		output, err := DecodeStageOutput(stage, resp)
		if err != nil {
			result.ErrorText = err.Error()
			result.FinishedAt = time.Now().UTC()
			emitStageEvent(r.Options.Telemetry, pipelineEventStageDecodeError, taskID, stage.Name(), err.Error(), map[string]any{
				"stage_index":   index,
				"retry_attempt": attempt,
			})
			if contract.Metadata.RetryPolicy.RetryOnDecodeError && attempt < maxRetries {
				result.Transition = StageTransition{Kind: TransitionRetry, Reason: err.Error()}
				continue
			}
			result.Transition = StageTransition{Kind: TransitionStop, Reason: err.Error()}
			return result, err
		}
		result.DecodedOutput = output

		if err := ValidateStageOutput(stage, output); err != nil {
			result.ErrorText = err.Error()
			result.FinishedAt = time.Now().UTC()
			emitStageEvent(r.Options.Telemetry, pipelineEventStageValidError, taskID, stage.Name(), err.Error(), map[string]any{
				"stage_index":   index,
				"retry_attempt": attempt,
			})
			if contract.Metadata.RetryPolicy.RetryOnValidationError && attempt < maxRetries {
				result.Transition = StageTransition{Kind: TransitionRetry, Reason: err.Error()}
				continue
			}
			result.Transition = StageTransition{Kind: TransitionStop, Reason: err.Error()}
			return result, err
		}
		result.ValidationOK = true

		if err := ApplyStageOutput(stage, state, output); err != nil {
			result.ErrorText = err.Error()
			result.FinishedAt = time.Now().UTC()
			result.Transition = StageTransition{Kind: TransitionStop, Reason: err.Error()}
			return result, err
		}
		result.ErrorText = ""
		result.Transition = StageTransition{Kind: TransitionNext}
		break
	}
	result.FinishedAt = time.Now().UTC()
	emitStageEvent(r.Options.Telemetry, pipelineEventStageFinish, taskID, stage.Name(), "", map[string]any{
		"stage_index":   index,
		"validation_ok": true,
		"transition":    result.Transition.Kind,
		"retry_attempt": result.RetryAttempt,
	})
	return result, nil
}

func (r *Runner) generateStageResponse(ctx context.Context, task *core.Task, state *core.Context, stage Stage, prompt string, stageTools []core.Tool) (*core.LLMResponse, bool, error) {
	if len(stageTools) == 0 || !r.Options.EnableToolCalling || !stage.Contract().Metadata.AllowTools {
		resp, err := r.Options.Model.Generate(ctx, prompt, &core.LLMOptions{
			Model: r.Options.ModelName,
		})
		return resp, false, err
	}
	resp, err := r.Options.Model.ChatWithTools(ctx, []core.Message{
		{
			Role:    "user",
			Content: prompt,
		},
	}, stageTools, &core.LLMOptions{
		Model: r.Options.ModelName,
	})
	if err != nil {
		return nil, false, err
	}
	calls := resp.ToolCalls
	if len(calls) == 0 {
		calls = toolsys.ParseToolCallsFromText(resp.Text)
	}
	if len(calls) == 0 && requiresToolExecution(stage, task, state, stageTools) {
		retryPrompt := prompt + "\n\nYou must call at least one allowed tool that verifies the task before you return the final JSON."
		resp, err = r.Options.Model.ChatWithTools(ctx, []core.Message{
			{
				Role:    "user",
				Content: retryPrompt,
			},
		}, stageTools, &core.LLMOptions{
			Model: r.Options.ModelName,
		})
		if err != nil {
			return nil, false, err
		}
		calls = resp.ToolCalls
		if len(calls) == 0 {
			calls = toolsys.ParseToolCallsFromText(resp.Text)
		}
	}
	if len(calls) == 0 {
		return resp, false, nil
	}
	observations, err := executeToolCalls(ctx, state, calls, stageTools)
	if err != nil {
		return nil, false, err
	}
	finalPrompt := prompt + "\n\nTool results:\n" + formatToolObservations(observations) + "\n\nUse the tool results above and return ONLY the final JSON for this stage."
	resp, err = r.Options.Model.Generate(ctx, finalPrompt, &core.LLMOptions{
		Model: r.Options.ModelName,
	})
	return resp, true, err
}

func requiresToolExecution(stage Stage, task *core.Task, state *core.Context, tools []core.Tool) bool {
	required, ok := stage.(ToolRequiredStage)
	if !ok {
		return false
	}
	return required.RequiresToolExecution(task, state, tools)
}
