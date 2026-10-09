package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	execution "codeburg.org/lexbit/relurpify/execution"
)

// ErrResumeBusy is returned when a resume is attempted while another run is
// executing. Resume never queue-jumps an active run.
var ErrResumeBusy = errors.New("resume refused: a run is already active")

// ResumeOutcome reports a completed resume: the executed continuation task
// and the workflow it resumed from.
type ResumeOutcome struct {
	TaskID     string
	WorkflowID string
}

// resumeSeed is what a resume continues from. Path A: a lifecycle workflow
// record exists (seeded by RunTask) and supplies the prior task. Path B:
// transcript only — the runtime holds no transcript, so the caller must
// supply a follow-up instruction. The checkpoint-based upgrade path will
// replace the transcript-tail seed with a checkpoint reference; this struct
// is the seam for exactly that swap.
type resumeSeed struct {
	workflowID string
	priorTask  string
}

// ResumeSession continues a previous session with a real executed turn,
// never a phantom outcome: it loads the prior session record, constructs the
// continuation task (the caller's follow-up when supplied, else a
// re-grounding instruction from the prior task), and executes it through the
// same internal path RunTask uses — events stream to the active surface via
// the runtime telemetry. Resuming while a run is active fails with
// ErrResumeBusy.
func (r *Runtime) ResumeSession(ctx context.Context, workflowID, followUp string) (*ResumeOutcome, error) {
	if r == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	if current := r.ActiveWorkflowID(); current != "" {
		return nil, fmt.Errorf("%w: workflow %s", ErrResumeBusy, current)
	}
	seed, err := r.resumeSeed(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	instruction := strings.TrimSpace(followUp)
	if instruction == "" {
		if seed.priorTask == "" {
			return nil, fmt.Errorf("resume requires a follow-up instruction (no prior task recorded for workflow %q)", workflowID)
		}
		instruction = fmt.Sprintf(
			"Continue the prior task: %s. Assess the current state, complete what remains, and report the outcome.",
			seed.priorTask)
	}
	task := &execution.Task{
		ID:          fmt.Sprintf("resume-%d", time.Now().UnixNano()),
		Instruction: instruction,
		Type:        string(execution.TaskTypeExecute),
		Metadata:    map[string]any{"resume_workflow": seed.workflowID},
	}
	if _, err := r.executeTask(ctx, task); err != nil {
		return nil, err
	}
	return &ResumeOutcome{TaskID: task.ID, WorkflowID: seed.workflowID}, nil
}

// resumeSeed builds the continuation seed. An empty workflowID is the
// transcript-only path (path B); a named workflow that no longer exists is a
// caller error, not a silent fresh start.
func (r *Runtime) resumeSeed(ctx context.Context, workflowID string) (resumeSeed, error) {
	seed := resumeSeed{}
	if strings.TrimSpace(workflowID) == "" {
		return seed, nil
	}
	if r.AgentLifecycle == nil {
		return seed, fmt.Errorf("workflow %q not found: lifecycle repository unavailable", workflowID)
	}
	workflow, err := r.AgentLifecycle.GetWorkflow(ctx, workflowID)
	if err != nil || workflow == nil {
		return seed, fmt.Errorf("workflow %q not found", workflowID)
	}
	seed.workflowID = workflow.WorkflowID
	if workflow.Metadata != nil {
		if instruction, ok := workflow.Metadata["instruction"].(string); ok {
			seed.priorTask = strings.TrimSpace(instruction)
		}
	}
	return seed, nil
}
