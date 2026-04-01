package modes

import (
	"context"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// ChatIntentPhase classifies the chat request as ask (read-only) or implement
// (mutation permitted) and surfaces the interpretation for user confirmation.
type ChatIntentPhase struct{}

func (p *ChatIntentPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	instruction, _ := mc.State["instruction"].(string)
	subMode := classifyChatSubMode(instruction)

	content := interaction.ProposalContent{
		Interpretation: instruction,
		Approach:       "chat_" + subMode,
	}
	if subMode == "ask" {
		content.Constraints = []string{"read-only: no file changes"}
	}

	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameProposal,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: content,
		Actions: []interaction.ActionSlot{
			{ID: "confirm", Label: "Answer this", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
			{ID: "implement", Label: "Implement instead", Kind: interaction.ActionConfirm},
			{ID: "clarify", Label: "Rephrase question", Kind: interaction.ActionFreetext},
		},
		Continuable: true,
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}

	if err := mc.Emitter.Emit(ctx, frame); err != nil {
		return interaction.PhaseOutcome{}, err
	}

	resp, err := mc.Emitter.AwaitResponse(ctx)
	if err != nil {
		return interaction.PhaseOutcome{}, err
	}

	updates := map[string]any{
		"intent.response": resp.ActionID,
		"chat.sub_mode":   subMode,
	}

	switch resp.ActionID {
	case "implement":
		updates["chat.sub_mode"] = "implement"
	case "clarify":
		if resp.Text != "" {
			updates["instruction"] = resp.Text
		}
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

// ChatPresentPhase delivers the answer or implements the request.
// For ask sub-mode: read-only analysis via a react task.
// For implement sub-mode: delegates to a code execution recipe.
type ChatPresentPhase struct {
	// RunAnalysis is an optional callback for executing the analysis.
	RunAnalysis func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.SummaryContent, error)
}

func (p *ChatPresentPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	statusFrame := interaction.InteractionFrame{
		Kind:  interaction.FrameStatus,
		Mode:  mc.Mode,
		Phase: mc.Phase,
		Content: interaction.StatusContent{
			Message: "Thinking...",
			Phase:   mc.Phase,
		},
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}
	if err := mc.Emitter.Emit(ctx, statusFrame); err != nil {
		return interaction.PhaseOutcome{}, err
	}

	var summary interaction.SummaryContent
	if p.RunAnalysis != nil {
		var err error
		summary, err = p.RunAnalysis(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		instruction, _ := mc.State["instruction"].(string)
		summary = interaction.SummaryContent{
			Description: instruction,
		}
	}

	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameResult,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: summary,
		Actions: []interaction.ActionSlot{
			{ID: "done", Label: "Done", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
			{ID: "follow_up", Label: "Follow-up question", Kind: interaction.ActionFreetext},
			{ID: "implement", Label: "Implement this", Kind: interaction.ActionConfirm,
				CapabilityTrigger: "euclo:chat.propose_transition"},
		},
		Continuable: true,
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}

	if err := mc.Emitter.Emit(ctx, frame); err != nil {
		return interaction.PhaseOutcome{}, err
	}

	resp, err := mc.Emitter.AwaitResponse(ctx)
	if err != nil {
		return interaction.PhaseOutcome{}, err
	}

	updates := map[string]any{
		"present.response": resp.ActionID,
		"present.summary":  summary,
		"present.answered": true,
	}

	if resp.ActionID == "follow_up" && resp.Text != "" {
		updates["present.follow_up"] = resp.Text
		delete(updates, "present.answered")
	}
	if resp.ActionID == "implement" {
		updates["chat.propose_transition"] = "code"
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

// ChatReflectPhase proposes next steps based on the chat outcome.
// Skipped for simple ask interactions.
type ChatReflectPhase struct{}

func (p *ChatReflectPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	summary, _ := mc.State["present.summary"].(interaction.SummaryContent)

	var transitionTarget string
	if t, ok := mc.State["chat.propose_transition"].(string); ok {
		transitionTarget = t
	}

	actions := []interaction.ActionSlot{
		{ID: "done", Label: "Done", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
		{ID: "implement", Label: "Implement this", Kind: interaction.ActionConfirm},
		{ID: "follow_up", Label: "Ask another question", Kind: interaction.ActionFreetext},
	}

	var content interface{} = summary
	if transitionTarget != "" {
		content = interaction.TransitionContent{
			FromMode: "chat",
			ToMode:   transitionTarget,
			Reason:   "user requested implementation",
		}
	}

	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameResult,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: content,
		Actions: actions,
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}

	if err := mc.Emitter.Emit(ctx, frame); err != nil {
		return interaction.PhaseOutcome{}, err
	}

	resp, err := mc.Emitter.AwaitResponse(ctx)
	if err != nil {
		return interaction.PhaseOutcome{}, err
	}

	updates := map[string]any{
		"reflect.response": resp.ActionID,
	}

	if resp.ActionID == "follow_up" && resp.Text != "" {
		updates["reflect.follow_up"] = resp.Text
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

// classifyChatSubMode determines whether an instruction is a read-only ask
// or an implementation request.
func classifyChatSubMode(instruction string) string {
	lower := strings.ToLower(instruction)
	implementPatterns := []string{
		"implement", "write", "create", "build", "add", "fix", "change", "update", "refactor",
	}
	for _, p := range implementPatterns {
		if strings.Contains(lower, p) {
			return "implement"
		}
	}
	return "ask"
}
