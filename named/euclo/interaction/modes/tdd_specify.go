package modes

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// BehaviorSpecPhase elicits behavior specifications from the user through
// targeted questions about happy paths, edge cases, and error cases.
type BehaviorSpecPhase struct {
	// GenerateQuestions is an optional callback for LLM-driven question generation
	// based on function signatures, existing tests, and uncovered code paths.
	// If nil, default behavior elicitation questions are used.
	GenerateQuestions func(ctx context.Context, mc interaction.PhaseMachineContext) ([]interaction.QuestionContent, error)
}

func (p *BehaviorSpecPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	// Load or create spec.
	spec := loadOrCreateSpec(mc)

	var questions []interaction.QuestionContent
	if p.GenerateQuestions != nil {
		var err error
		questions, err = p.GenerateQuestions(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		questions = defaultBehaviorQuestions()
	}

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
		if q.AllowFreetext {
			actions = append(actions, interaction.ActionSlot{
				ID:    "freetext",
				Label: "Describe behavior",
				Kind:  interaction.ActionFreetext,
			})
		}
		actions = append(actions,
			interaction.ActionSlot{ID: "skip", Label: "Skip, use defaults", Kind: interaction.ActionConfirm},
			interaction.ActionSlot{ID: "matrix", Label: "Show spec matrix", Kind: interaction.ActionConfirm},
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

		if resp.ActionID == "matrix" {
			// Emit the spec matrix as a draft frame (non-advancing).
			if err := emitSpecMatrix(ctx, mc, spec); err != nil {
				return interaction.PhaseOutcome{}, err
			}
			// Continue to next question after showing matrix.
			continue
		}

		// Accumulate the answer into the spec.
		accumulateBehavior(&spec, q.Question, resp)
	}

	updates := map[string]any{
		"specify.spec":           spec,
		"specify.total_cases":    spec.TotalCases(),
		"specify.question_count": len(questions),
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

func loadOrCreateSpec(mc interaction.PhaseMachineContext) BehaviorSpec {
	if existing, ok := mc.State["specify.spec"].(BehaviorSpec); ok {
		return existing
	}
	target, _ := mc.State["function_target"].(string)
	return BehaviorSpec{FunctionTarget: target}
}

func accumulateBehavior(spec *BehaviorSpec, question string, resp interaction.UserResponse) {
	bc := BehaviorCase{Description: resp.Text}
	if bc.Description == "" {
		bc.Description = resp.ActionID
	}

	// Categorize based on question content (simple heuristic).
	switch {
	case containsAny(question, "error", "fail", "invalid", "reject"):
		spec.ErrorCases = append(spec.ErrorCases, bc)
	case containsAny(question, "edge", "boundary", "limit", "empty", "zero"):
		spec.EdgeCases = append(spec.EdgeCases, bc)
	default:
		spec.HappyPaths = append(spec.HappyPaths, bc)
	}
}

func containsAny(s string, substrs ...string) bool {
	lower := toLower(s)
	for _, sub := range substrs {
		if containsLower(lower, sub) {
			return true
		}
	}
	return false
}

// toLower and containsLower avoid importing strings for minimal helpers.
func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func containsLower(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func emitSpecMatrix(ctx context.Context, mc interaction.PhaseMachineContext, spec BehaviorSpec) error {
	items := make([]interaction.DraftItem, 0)
	for i, c := range spec.HappyPaths {
		items = append(items, interaction.DraftItem{
			ID:      fmt.Sprintf("happy-%d", i+1),
			Content: fmt.Sprintf("[Happy] %s", c.Description),
		})
	}
	for i, c := range spec.EdgeCases {
		items = append(items, interaction.DraftItem{
			ID:      fmt.Sprintf("edge-%d", i+1),
			Content: fmt.Sprintf("[Edge] %s", c.Description),
		})
	}
	for i, c := range spec.ErrorCases {
		items = append(items, interaction.DraftItem{
			ID:      fmt.Sprintf("error-%d", i+1),
			Content: fmt.Sprintf("[Error] %s", c.Description),
		})
	}

	frame := interaction.InteractionFrame{
		Kind:  interaction.FrameDraft,
		Mode:  mc.Mode,
		Phase: mc.Phase,
		Content: interaction.DraftContent{
			Kind:  "test_list",
			Items: items,
		},
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}
	return mc.Emitter.Emit(ctx, frame)
}

func defaultBehaviorQuestions() []interaction.QuestionContent {
	return []interaction.QuestionContent{
		{
			Question: "What's the happy path behavior?",
			Options: []interaction.QuestionOption{
				{ID: "standard", Label: "Standard input produces expected output"},
			},
			AllowFreetext: true,
		},
		{
			Question: "Any edge cases to cover?",
			Options: []interaction.QuestionOption{
				{ID: "empty", Label: "Empty input"},
				{ID: "boundary", Label: "Boundary values"},
				{ID: "large", Label: "Large input"},
			},
			AllowFreetext: true,
		},
		{
			Question: "What error conditions should be tested?",
			Options: []interaction.QuestionOption{
				{ID: "invalid_input", Label: "Invalid input"},
				{ID: "nil", Label: "Nil/zero values"},
				{ID: "timeout", Label: "Timeout/resource exhaustion"},
			},
			AllowFreetext: true,
		},
	}
}
