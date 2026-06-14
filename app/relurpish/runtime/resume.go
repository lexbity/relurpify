package runtime

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
)

type ResumeOutcome struct {
	TaskID       string
	Envelope     *contextdata.Envelope
	PendingFrame *interaction.InteractionFrame
	RestoredMode string
	HasBKC       bool
}

func (r *Runtime) ResumeSession(ctx context.Context, workflowID string) (*ResumeOutcome, error) {
	if r == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	if workflowID == "" {
		return nil, fmt.Errorf("workflow ID required")
	}

	workflow, err := r.AgentLifecycle.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("get workflow %q: %w", workflowID, err)
	}
	if workflow == nil {
		return nil, fmt.Errorf("workflow %q not found", workflowID)
	}

	instruction := ""
	if workflow.Metadata != nil {
		if v, ok := workflow.Metadata["instruction"]; ok {
			instruction, _ = v.(string)
		}
	}
	if instruction == "" {
		instruction = fmt.Sprintf("Resume workflow %s", workflowID)
	}

	task := &execution.Task{
		ID:          fmt.Sprintf("resume-%s-%d", workflowID, time.Now().UnixNano()),
		Instruction: instruction,
		Type:        string(execution.TaskTypeExecute),
		Metadata:    workflow.Metadata,
	}

	env := contextdata.NewEnvelope(task.ID, "")
	env.SetWorkingValueWithClass(euclostate.KeyTaskInput, task, contextdata.MemoryClassTask)

	pendingFrame, _ := interaction.ResumeFrame(env)
	restoredMode := ""
	if workflow.Metadata != nil {
		if v, ok := workflow.Metadata["mode"]; ok {
			restoredMode, _ = v.(string)
		}
	}

	r.trackInteractionEnvelope(task.ID, env)

	return &ResumeOutcome{
		TaskID:       task.ID,
		Envelope:     env,
		PendingFrame: pendingFrame,
		RestoredMode: restoredMode,
	}, nil
}
