package agenttest

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// ErrUnansweredFrame is the typed case failure when a run paused on a DSL ask
// frame the case's interaction_script does not answer (D8): a paused run with
// an exhausted script never silently passes.
type ErrUnansweredFrame struct {
	TaskID       string
	FrameID      string
	Question     string
	ScriptLength int
}

func (e *ErrUnansweredFrame) Error() string {
	return fmt.Sprintf("unanswered_frame: run paused on interaction frame %q (question %q) with no interaction_script entry left (%d scripted)",
		e.FrameID, e.Question, e.ScriptLength)
}

// interactionFrameRecord is one (frame, answer) pair the driver executed,
// reported into the case report's frames section.
type interactionFrameRecord struct {
	FrameID  string `json:"frame_id"`
	Question string `json:"question,omitempty"`
	Answer   string `json:"answer"`
	Resumed  bool   `json:"resumed"`
}

// driveInteractionScript runs the case's interaction_script against a paused
// run (D8). The pause is detected from the RUN RESULT's pause metadata — the
// driver is composition-level and never reaches into the envelope's internal
// bookkeeping; the envelope is only read for the pending frame the ask node
// wrote, which is the same durable frame a surface would answer. Each paused
// frame consumes the next unconsumed script entry (Action is the answer,
// Text optional freetext); the executor re-executes and the recipe resumes at
// its stored resume node. Bounds: max rounds = len(script), so a re-pausing
// run with an exhausted script fails typed ErrUnansweredFrame instead of
// looping forever. Every (frame, answer) pair is recorded for the case report.
func (e *PreparedRunExecutor) driveInteractionScript(ctx context.Context, desc *PreparedRunDescriptor, task *execution.Task, env *contextdata.Envelope, result *execution.Result, out io.Writer) (*execution.Result, []interactionFrameRecord, error) {
	var records []interactionFrameRecord
	script := desc.InteractionScript
	if result == nil || !executionPaused(result) {
		return result, records, nil
	}
	exec := e.currentExecutor()
	for round := 0; round < len(script); round++ {
		if !executionPaused(result) {
			return result, records, nil
		}
		frame := pendingInteractionFrame(env)
		if frame == nil {
			return result, records, fmt.Errorf("run reported a pause but no pending interaction frame is on the envelope (task %q)", task.ID)
		}
		step := script[round]
		answer := strings.TrimSpace(step.Action)
		if answer == "" {
			answer = strings.TrimSpace(step.Text)
		}
		question := strings.TrimSpace(frame.Question)
		extra := map[string]any{
			"task_id":      strings.TrimSpace(task.ID),
			"frame_id":     strings.TrimSpace(frame.ID),
			"frame_type":   string(frame.Type),
			"resolved_via": "agenttest",
		}
		if strings.TrimSpace(step.Text) != "" {
			extra["freetext"] = strings.TrimSpace(step.Text)
		}
		// Write-once answer, same door a surface uses: the frame carries the
		// response onto the live envelope, then the agent re-executes and the
		// recipe executor re-enters at the stored resume node.
		frame.SetResponse(answer, extra, "agenttest", time.Now().UTC())
		records = append(records, interactionFrameRecord{
			FrameID:  strings.TrimSpace(frame.ID),
			Question: question,
			Answer:   answer,
		})
		resumed, err := exec.Execute(ctx, task, env)
		if err != nil {
			return result, records, fmt.Errorf("interaction resume %d failed: %w", round+1, err)
		}
		if resumed != nil {
			result = resumed
			records[len(records)-1].Resumed = resumed.Success
		}
	}
	if executionPaused(result) {
		frame := pendingInteractionFrame(env)
		if frame == nil {
			return result, records, &ErrUnansweredFrame{TaskID: task.ID, ScriptLength: len(script)}
		}
		return result, records, &ErrUnansweredFrame{
			TaskID: task.ID, FrameID: strings.TrimSpace(frame.ID),
			Question: strings.TrimSpace(frame.Question), ScriptLength: len(script),
		}
	}
	return result, records, nil
}

// executionPaused reports the ask node's pause metadata on the run result.
func executionPaused(result *execution.Result) bool {
	if result == nil || result.Metadata == nil {
		return false
	}
	pause, _ := result.Metadata["euclo.interaction.pause"].(bool)
	return pause
}

// pendingInteractionFrame finds the first interaction frame on the envelope
// that carries no response — the frame the surface would answer.
func pendingInteractionFrame(env *contextdata.Envelope) *interaction.InteractionFrame {
	if env == nil {
		return nil
	}
	var pending *interaction.InteractionFrame
	for _, key := range env.WorkingMemoryKeys() {
		value, ok := contextdata.GetTyped[any](env, key)
		if !ok {
			continue
		}
		frame, ok := value.(*interaction.InteractionFrame)
		if !ok || frame == nil || frame.Response != nil {
			continue
		}
		if pending == nil || strings.TrimSpace(frame.ID) < strings.TrimSpace(pending.ID) {
			pending = frame
		}
	}
	return pending
}
