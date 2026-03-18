package modes

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// ClarifyPhase emits targeted questions to resolve ambiguity before planning.
// Skipped when scope was confirmed without corrections.
type ClarifyPhase struct {
	// GenerateQuestions is an optional callback for LLM-driven question generation.
	// If nil, no questions are asked and the phase auto-advances.
	GenerateQuestions func(ctx context.Context, mc interaction.PhaseMachineContext) ([]interaction.QuestionContent, error)
}

func (p *ClarifyPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	var questions []interaction.QuestionContent
	if p.GenerateQuestions != nil {
		var err error
		questions, err = p.GenerateQuestions(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	}

	// Cap at 3 questions per spec.
	if len(questions) > 3 {
		questions = questions[:3]
	}

	updates := map[string]any{}
	answers := make([]map[string]any, 0, len(questions))

	for i, q := range questions {
		actions := buildQuestionActions(q, i)

		frame := interaction.InteractionFrame{
			Kind:    interaction.FrameQuestion,
			Mode:    mc.Mode,
			Phase:   mc.Phase,
			Content: q,
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

		// "skip" skips remaining questions.
		if resp.ActionID == "skip" {
			break
		}

		answer := map[string]any{
			"question":  q.Question,
			"action_id": resp.ActionID,
		}
		if resp.Text != "" {
			answer["text"] = resp.Text
		}
		answers = append(answers, answer)
	}

	updates["clarify.answers"] = answers
	updates["clarify.question_count"] = len(questions)

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

// buildQuestionActions creates action slots for a clarify question.
func buildQuestionActions(q interaction.QuestionContent, questionIndex int) []interaction.ActionSlot {
	actions := make([]interaction.ActionSlot, 0, len(q.Options)+2)

	for i, opt := range q.Options {
		actions = append(actions, interaction.ActionSlot{
			ID:       opt.ID,
			Label:    opt.Label,
			Kind:     interaction.ActionSelect,
			Default:  i == 0,
			Shortcut: fmt.Sprintf("%d", i+1),
		})
	}

	if q.AllowFreetext {
		actions = append(actions, interaction.ActionSlot{
			ID:    "freetext",
			Label: "Type answer",
			Kind:  interaction.ActionFreetext,
		})
	}

	actions = append(actions, interaction.ActionSlot{
		ID:    "skip",
		Label: "Skip remaining questions",
		Kind:  interaction.ActionConfirm,
	})

	// If no options and no freetext, make skip the default.
	if len(q.Options) == 0 && !q.AllowFreetext && len(actions) > 0 {
		actions[len(actions)-1].Default = true
	}

	_ = questionIndex // available for future per-question customization
	return actions
}
