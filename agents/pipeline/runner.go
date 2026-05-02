package pipeline

import (
	"context"
	"errors"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	frameworktools "codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// RunnerOptions configures how a pipeline executes stages.
type RunnerOptions struct {
	Model               contracts.LanguageModel
	ModelName           string
	Tools               []contracts.Tool
	EnableToolCalling   bool
	AgentSpec           *agentspec.AgentRuntimeSpec
	BackendCapabilities contracts.BackendCapabilities
	Telemetry           core.Telemetry
	CapabilityInvoker   CapabilityInvoker
}

// Runner executes a linear sequence of typed stages.
type Runner struct {
	Options RunnerOptions
}

// Execute runs the provided stages in order, optionally resuming from a checkpoint.
func (r *Runner) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope, stages []Stage) ([]StageResult, error) {
	if len(stages) == 0 {
		return nil, errors.New("pipeline stages required")
	}
	if r.Options.Model == nil {
		return nil, errors.New("pipeline runner model required")
	}
	if env == nil {
		env = contextdata.NewEnvelope("pipeline", "session")
	}
	taskID := ""
	if task != nil {
		taskID = task.ID
	}
	startIndex, env, priorResults, err := r.resume(taskID, env)
	if err != nil {
		return nil, err
	}

	results := append([]StageResult{}, priorResults...)
	for idx := startIndex; idx < len(stages); idx++ {
		stage := stages[idx]
		if err := ValidateStage(stage); err != nil {
			return results, err
		}

		stageResult, err := r.executeStage(ctx, task, taskID, env, stage, idx)
		results = append(results, stageResult)
		if err != nil {
			return results, err
		}
		if stageResult.Transition.Kind == TransitionStop {
			break
		}
	}
	return results, nil
}

func (r *Runner) resume(taskID string, env *contextdata.Envelope) (int, *contextdata.Envelope, []StageResult, error) {
	return 0, env, nil, nil
}

func (r *Runner) executeStage(ctx context.Context, task *core.Task, taskID string, env *contextdata.Envelope, stage Stage, index int) (StageResult, error) {
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
	stageTools := resolveStageTools(ctx, env, stage, r.Options.Tools)
	for attempt := 0; attempt <= maxRetries; attempt++ {
		result.RetryAttempt = attempt

		prompt, err := stage.BuildPrompt(env)
		if err != nil {
			result.ErrorText = err.Error()
			result.FinishedAt = time.Now().UTC()
			return result, err
		}
		result.Prompt = prompt

		resp, usedTools, err := r.generateStageResponse(ctx, task, env, stage, prompt, stageTools)
		if err != nil {
			result.ErrorText = err.Error()
			result.FinishedAt = time.Now().UTC()
			return result, err
		}
		result.Response = resp
		if requiresToolExecution(stage, task, env, stageTools) && !usedTools {
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

		if err := ApplyStageOutput(stage, env, output); err != nil {
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

func (r *Runner) generateStageResponse(ctx context.Context, task *core.Task, env *contextdata.Envelope, stage Stage, prompt string, stageTools []contracts.Tool) (*contracts.LLMResponse, bool, error) {
	if len(stageTools) == 0 || !r.Options.EnableToolCalling || !stage.Contract().Metadata.AllowTools {
		resp, err := r.Options.Model.Generate(ctx, prompt, &contracts.LLMOptions{
			Model: r.Options.ModelName,
		})
		return resp, false, err
	}

	mode := frameworktools.ResolveCallingMode(r.Options.AgentSpec, r.callingCapabilities())
	var (
		resp  *contracts.LLMResponse
		err   error
		calls []contracts.ToolCall
	)
	if mode == frameworktools.CapabilityCallingNative {
		resp, calls, err = r.nativeToolCall(ctx, prompt, stageTools)
		if err == nil && len(calls) == 0 && requiresToolExecution(stage, task, env, stageTools) {
			resp, calls, err = r.nativeRetryToolCall(ctx, prompt, stageTools, stage, task, env)
		}
	} else {
		resp, calls, err = r.fallbackToolCall(ctx, prompt, stageTools, stage, task, env)
	}
	if err != nil {
		return nil, false, err
	}
	if len(calls) == 0 {
		return resp, false, nil
	}
	observations, err := executeToolCalls(ctx, env, calls, stageTools, r.Options.CapabilityInvoker)
	if err != nil {
		return nil, false, err
	}
	finalPrompt := prompt + "\n\nTool results:\n" + formatToolObservations(observations) + "\n\nUse the tool results above and return ONLY the final JSON for this stage."
	resp, err = r.Options.Model.Generate(ctx, finalPrompt, &contracts.LLMOptions{
		Model: r.Options.ModelName,
	})
	return resp, true, err
}

func (r *Runner) callingCapabilities() contracts.BackendCapabilities {
	caps := r.Options.BackendCapabilities
	if pm, ok := r.Options.Model.(contracts.ProfiledModel); ok {
		caps.NativeToolCalling = caps.NativeToolCalling && pm.UsesNativeToolCalling()
	}
	return caps
}

func (r *Runner) nativeToolCall(ctx context.Context, prompt string, stageTools []contracts.Tool) (*contracts.LLMResponse, []contracts.ToolCall, error) {
	toolSpecs := contracts.LLMToolSpecsFromTools(stageTools)
	resp, err := r.Options.Model.ChatWithTools(ctx, []contracts.Message{{
		Role:    "user",
		Content: prompt,
	}}, toolSpecs, &contracts.LLMOptions{
		Model: r.Options.ModelName,
	})
	if err != nil {
		return nil, nil, err
	}
	return resp, collectToolCalls(resp), nil
}

func (r *Runner) nativeRetryToolCall(ctx context.Context, prompt string, stageTools []contracts.Tool, stage Stage, task *core.Task, env *contextdata.Envelope) (*contracts.LLMResponse, []contracts.ToolCall, error) {
	toolSpecs := contracts.LLMToolSpecsFromTools(stageTools)
	retryPrompt := prompt + "\n\nYou must call at least one allowed verification tool before returning the final JSON. Do not summarize hypothetical results. Return a tool call now, not the final report."
	resp, err := r.Options.Model.ChatWithTools(ctx, []contracts.Message{{
		Role:    "user",
		Content: retryPrompt,
	}}, toolSpecs, &contracts.LLMOptions{
		Model: r.Options.ModelName,
	})
	if err != nil {
		return nil, nil, err
	}
	calls := collectToolCalls(resp)
	if len(calls) == 0 && requiresToolExecution(stage, task, env, stageTools) {
		return resp, calls, nil
	}
	return resp, calls, nil
}

func (r *Runner) fallbackToolCall(ctx context.Context, prompt string, stageTools []contracts.Tool, stage Stage, task *core.Task, env *contextdata.Envelope) (*contracts.LLMResponse, []contracts.ToolCall, error) {
	renderedPrompt := prompt + "\n\n" + frameworktools.RenderToolsToPrompt(stageTools)
	resp, err := r.Options.Model.Generate(ctx, renderedPrompt, &contracts.LLMOptions{
		Model: r.Options.ModelName,
	})
	if err != nil {
		return nil, nil, err
	}
	calls := collectToolCalls(resp)
	if len(calls) == 0 && requiresToolExecution(stage, task, env, stageTools) {
		retryPrompt := renderedPrompt + "\n\nYou must call at least one allowed verification tool before returning the final JSON. Do not summarize hypothetical results. Return a tool call now, not the final report."
		resp, err = r.Options.Model.Generate(ctx, retryPrompt, &contracts.LLMOptions{
			Model: r.Options.ModelName,
		})
		if err != nil {
			return nil, nil, err
		}
		calls = collectToolCalls(resp)
	}
	return resp, calls, nil
}

func collectToolCalls(resp *contracts.LLMResponse) []contracts.ToolCall {
	if resp == nil {
		return nil
	}
	calls := append([]contracts.ToolCall(nil), resp.ToolCalls...)
	if len(calls) == 0 {
		calls = frameworktools.ParseToolCallsFromText(resp.Text)
	}
	return calls
}

func requiresToolExecution(stage Stage, task *core.Task, env *contextdata.Envelope, tools []contracts.Tool) bool {
	required, ok := stage.(ToolRequiredStage)
	if !ok {
		return false
	}
	return required.RequiresToolExecution(task, env, tools)
}
