package thoughtrecipe

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// AskNode executes an ask-user step by creating an interaction frame.
type AskNode struct {
	stepCore
}

// NewAskNode creates a new AskNode.
func NewAskNode(id string, deps *paradigm.Deps, step ExecutionStep) *AskNode {
	return &AskNode{stepCore: stepCore{id: id, deps: deps, step: step}}
}

// Type implements agentgraph.Node.
func (n *AskNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeTool }

// Execute creates or resumes an ask-user interaction frame.
func (n *AskNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if n == nil {
		return nil, fmt.Errorf("thoughtrecipe step node is nil")
	}

	start := time.Now()
	emitStepStarted(ctx, env, n.step)

	var stepResult *execution.Result
	var stepErr error
	defer func() {
		success := stepResult != nil && stepResult.Success && stepErr == nil
		dur := time.Since(start)
		emitStepCompleted(ctx, env, n.step, success, dur)
	}()

	frame, created := n.ensureAskFrame(env)
	if frame == nil {
		return nil, fmt.Errorf("ask step %q failed to initialize frame", n.step.ID)
	}

	if frame.Response == nil || frame.RespondedAt == nil {
		if created {
			if err := interaction.EmitFrame(ctx, frame, env, telemetry.TelemetryFromContext(ctx)); err != nil {
				return nil, err
			}
			contextdata.SetTyped(env, askFrameKey(n.step.ID), frame)
		}
		state.SetInteractionFrameRequested(env, true)
		state.SetInteractionResumeNodeID(env, n.id)
		stepResult = &execution.Result{
			NodeID:  n.id,
			Success: true,
			Data: execution.NewToolResultPayload(map[string]any{
				"paused":   true,
				"frame_id": frame.ID,
				"question": n.step.Question,
			}),
			Metadata: map[string]any{
				"euclo.interaction.pause": true,
				"frame_id":                frame.ID,
			},
		}
		return stepResult, nil
	}

	answer, _ := interaction.ResponseValue(frame)
	stepResult = &execution.Result{
		NodeID:  n.id,
		Success: true,
		Data: execution.NewToolResultPayload(map[string]any{
			"answer":        answer,
			"selected_slot": answer,
			"frame_id":      frame.ID,
		}),
	}
	if frame.Response != nil && len(frame.Response.ExtraData) > 0 {
		fields := execution.ResultFields(stepResult.Data)
		fields["response"] = frame.Response.ExtraData
		stepResult.Data = execution.NewToolResultPayload(fields)
	}
	if err := n.writeCaptures(env, stepResult); err != nil {
		stepErr = err
		return stepResult, err
	}
	n.writeStepMetadata(env)
	state.SetInteractionResumeNodeID(env, "")
	state.SetInteractionFrameRequested(env, false)
	contextdata.SetTyped(env, askFrameKey(n.step.ID), frame)
	contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".result", stepResult.Data)
	contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".success", true)
	return stepResult, nil
}

func (n *AskNode) ensureAskFrame(env *contextdata.Envelope) (*interaction.InteractionFrame, bool) {
	if n == nil {
		return nil, false
	}
	key := askFrameKey(n.step.ID)
	if frame, ok := contextdata.GetTyped[*interaction.InteractionFrame](env, key); ok && frame != nil {
		return frame, false
	}
	choices := n.askChoices(env)
	frame := interaction.NewAskUserFrame(env.TaskID, env.SessionID, n.step.Question, choices)
	frame.Payload["step_id"] = n.step.ID
	frame.Payload["step_type"] = n.step.Kind.String()
	frame.Payload["choice_source"] = n.step.ChoiceSource
	frame.Payload["frame_key"] = key
	contextdata.SetTyped(env, key, frame)
	return frame, true
}

func (n *AskNode) askChoices(env *contextdata.Envelope) []string {
	if n == nil {
		return nil
	}
	if len(n.step.Choices) > 0 {
		return append([]string(nil), n.step.Choices...)
	}
	if strings.TrimSpace(n.step.ChoiceSource) == "" {
		return nil
	}
	data := thoughtrecipeTemplateData(env, n.step)
	value, ok := lookupTemplateValue(data, n.step.ChoiceSource)
	if !ok {
		return nil
	}
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, entry := range v {
			if s := strings.TrimSpace(fmt.Sprint(entry)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if s := strings.TrimSpace(part); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
