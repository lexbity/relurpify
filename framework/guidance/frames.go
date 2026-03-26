package guidance

import (
	"time"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

func GuidanceRequestToFrame(req GuidanceRequest) interaction.InteractionFrame {
	options := make([]interaction.QuestionOption, 0, len(req.Choices))
	actions := make([]interaction.ActionSlot, 0, len(req.Choices)+1)
	for i, choice := range req.Choices {
		options = append(options, interaction.QuestionOption{
			ID:          choice.ID,
			Label:       choice.Label,
			Description: choice.Description,
		})
		actions = append(actions, interaction.ActionSlot{
			ID:       choice.ID,
			Label:    choice.Label,
			Shortcut: shortcutForIndex(i),
			Kind:     interaction.ActionSelect,
			Default:  choice.IsDefault,
		})
	}
	actions = append(actions, interaction.ActionSlot{
		ID:    "freetext",
		Label: "Other",
		Kind:  interaction.ActionFreetext,
	})

	return interaction.InteractionFrame{
		Kind:  interaction.FrameQuestion,
		Mode:  "guidance",
		Phase: string(req.Kind),
		Content: interaction.QuestionContent{
			Question:      req.Title,
			Description:   req.Description,
			Options:       options,
			AllowFreetext: true,
		},
		Actions:     actions,
		Continuable: false,
		Metadata: interaction.FrameMetadata{
			Timestamp: time.Now().UTC(),
		},
	}
}

func GuidanceDecisionFromResponse(req GuidanceRequest, resp interaction.UserResponse) GuidanceDecision {
	decision := GuidanceDecision{
		RequestID: req.ID,
		DecidedBy: "user",
		DecidedAt: time.Now().UTC(),
	}
	if resp.Text != "" {
		decision.Freetext = resp.Text
	}
	for _, choice := range req.Choices {
		if resp.ActionID == choice.ID {
			decision.ChoiceID = choice.ID
			return decision
		}
	}
	if decision.Freetext == "" && resp.ActionID != "" && resp.ActionID != "freetext" {
		decision.Freetext = resp.ActionID
	}
	return decision
}

func shortcutForIndex(i int) string {
	if i < 0 || i > 8 {
		return ""
	}
	return string(rune('1' + i))
}
