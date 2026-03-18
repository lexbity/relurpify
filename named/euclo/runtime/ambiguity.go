package runtime

import (
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// BuildAmbiguityFrame creates a FrameQuestion for ambiguous classification.
// The user selects which mode to use instead of the classifier guessing.
func BuildAmbiguityFrame(scored ScoredClassification) interaction.InteractionFrame {
	options := make([]interaction.QuestionOption, 0, len(scored.Candidates))
	actions := make([]interaction.ActionSlot, 0, len(scored.Candidates))

	for i, c := range scored.Candidates {
		if i >= 5 {
			break // Cap at 5 options.
		}
		desc := modeDescription(c.Mode)
		options = append(options, interaction.QuestionOption{
			ID:          c.Mode,
			Label:       modeLabel(c.Mode),
			Description: desc,
		})
		action := interaction.ActionSlot{
			ID:    c.Mode,
			Label: modeLabel(c.Mode),
			Kind:  interaction.ActionSelect,
		}
		if i == 0 {
			action.Default = true
		}
		actions = append(actions, action)
	}

	// Always include "plan first" as an option.
	hasPlan := false
	for _, o := range options {
		if o.ID == "planning" {
			hasPlan = true
			break
		}
	}
	if !hasPlan {
		options = append(options, interaction.QuestionOption{
			ID:          "planning",
			Label:       "Plan first",
			Description: "Step back and think about the approach",
		})
		actions = append(actions, interaction.ActionSlot{
			ID:    "planning",
			Label: "Plan first",
			Kind:  interaction.ActionSelect,
		})
	}

	question := fmt.Sprintf("I see this could be approached in %d ways. Which fits best?", len(options))

	return interaction.InteractionFrame{
		Kind:  interaction.FrameQuestion,
		Mode:  "classification",
		Phase: "disambiguate",
		Content: interaction.QuestionContent{
			Question: question,
			Options:  options,
		},
		Actions: actions,
		Metadata: interaction.FrameMetadata{
			Timestamp: time.Now(),
		},
	}
}

// ResolveAmbiguity takes the user's response to the ambiguity frame and
// returns the selected mode ID.
func ResolveAmbiguity(scored ScoredClassification, resp interaction.UserResponse) string {
	// Check if the response matches a candidate mode.
	if resp.ActionID != "" {
		for _, c := range scored.Candidates {
			if c.Mode == resp.ActionID {
				return c.Mode
			}
		}
		// Could be "planning" added as extra option.
		if resp.ActionID == "planning" {
			return "planning"
		}
	}
	// Fallback to top candidate.
	if len(scored.Candidates) > 0 {
		return scored.Candidates[0].Mode
	}
	return "code"
}

func modeLabel(mode string) string {
	switch mode {
	case "code":
		return "Code — make the change"
	case "debug":
		return "Debug — investigate first"
	case "planning":
		return "Plan — think about approach"
	case "tdd":
		return "TDD — write tests first"
	case "review":
		return "Review — inspect before changing"
	default:
		return mode
	}
}

func modeDescription(mode string) string {
	switch mode {
	case "code":
		return "Apply the change directly with verification"
	case "debug":
		return "Investigate the root cause before making changes"
	case "planning":
		return "Plan the approach before executing"
	case "tdd":
		return "Write tests first, then implement"
	case "review":
		return "Review the code before making changes"
	default:
		return ""
	}
}
