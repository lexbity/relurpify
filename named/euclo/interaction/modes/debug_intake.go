package modes

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// DiagnosticIntakePhase collects symptom information from the user
// before attempting reproduction. Asks up to 3 targeted questions.
type DiagnosticIntakePhase struct {
	// GenerateQuestions is an optional callback for generating diagnostic questions.
	// If nil, default symptom-collection questions are used.
	GenerateQuestions func(ctx context.Context, mc interaction.PhaseMachineContext) ([]interaction.QuestionContent, error)
}

func (p *DiagnosticIntakePhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	var questions []interaction.QuestionContent
	if p.GenerateQuestions != nil {
		var err error
		questions, err = p.GenerateQuestions(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		questions = defaultDiagnosticQuestions()
	}

	// Cap at 3 questions.
	if len(questions) > 3 {
		questions = questions[:3]
	}

	updates := map[string]any{}
	symptoms := make([]map[string]any, 0, len(questions))

	for _, q := range questions {
		actions := []interaction.ActionSlot{}
		for i, opt := range q.Options {
			actions = append(actions, interaction.ActionSlot{
				ID:      opt.ID,
				Label:   opt.Label,
				Kind:    interaction.ActionSelect,
				Default: i == 0,
			})
		}
		actions = append(actions,
			interaction.ActionSlot{ID: "paste", Label: "Paste error/trace", Kind: interaction.ActionFreetext},
			interaction.ActionSlot{ID: "test", Label: "Show failing test", Kind: interaction.ActionFreetext},
			interaction.ActionSlot{ID: "skip", Label: "Just fix it", Kind: interaction.ActionConfirm},
		)

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

		if resp.ActionID == "skip" {
			break
		}

		symptom := map[string]any{
			"question":  q.Question,
			"action_id": resp.ActionID,
		}
		if resp.Text != "" {
			symptom["text"] = resp.Text
		}
		symptoms = append(symptoms, symptom)
	}

	updates["intake.symptoms"] = symptoms
	updates["intake.question_count"] = len(questions)

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

func defaultDiagnosticQuestions() []interaction.QuestionContent {
	return []interaction.QuestionContent{
		{
			Question: "What behavior are you seeing vs. what you expect?",
			Options: []interaction.QuestionOption{
				{ID: "wrong_output", Label: "Wrong output", Description: "Function returns incorrect result"},
				{ID: "crash", Label: "Crash/panic", Description: "Program crashes or panics"},
				{ID: "test_fail", Label: "Test failure", Description: "One or more tests fail"},
				{ID: "hang", Label: "Hang/timeout", Description: "Program hangs or times out"},
			},
			AllowFreetext: true,
		},
		{
			Question: "Is this consistent or intermittent?",
			Options: []interaction.QuestionOption{
				{ID: "consistent", Label: "Always happens"},
				{ID: "intermittent", Label: "Sometimes happens"},
				{ID: "recent", Label: "Started recently"},
			},
			AllowFreetext: false,
		},
		{
			Question: "When did this start?",
			Options: []interaction.QuestionOption{
				{ID: "always", Label: "Always been broken"},
				{ID: "recent_change", Label: "After a recent change"},
				{ID: "unknown", Label: "Not sure"},
			},
			AllowFreetext: true,
		},
	}
}
