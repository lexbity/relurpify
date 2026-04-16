package pretask

import (
	"context"
	"fmt"
	"strings"
	"time"

	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	"github.com/lexcodex/relurpify/framework/core"
)

// SessionRevisionResolver extracts the last known session revision from state.
type SessionRevisionResolver func(state *core.Context) string

// LearningDeltaService exposes the subset of the learning service needed to
// compute the post-session delta summary.
type LearningDeltaService interface {
	ListByWorkflow(ctx context.Context, workflowID string) ([]archaeolearning.Interaction, error)
}

// LearningDeltaStep computes a summary of resolved learning interactions since
// the previous session boundary and injects it into the pretask context.
type LearningDeltaStep struct {
	LearningService  LearningDeltaService
	WorkflowResolver WorkflowIDResolver
	SessionResolver  SessionRevisionResolver
}

func (s LearningDeltaStep) ID() string {
	return "euclo:learning.delta"
}

func (s LearningDeltaStep) Run(ctx context.Context, state *core.Context) error {
	if state == nil || s.WorkflowResolver == nil || s.LearningService == nil {
		return nil
	}
	workflowID := strings.TrimSpace(s.WorkflowResolver(state))
	if workflowID == "" {
		return nil
	}
	if s.SessionResolver != nil {
		if revision := strings.TrimSpace(s.SessionResolver(state)); revision != "" {
			state.Set("euclo.last_session_revision", revision)
		}
	}
	lastSessionTime := stateTimeValue(state, "euclo.last_session_time")
	interactions, err := s.LearningService.ListByWorkflow(ctx, workflowID)
	if err != nil {
		return err
	}
	delta := summarizeLearningDelta(interactions, lastSessionTime)
	if delta.TotalResolved == 0 {
		return nil
	}
	state.Set("euclo.learning_delta", delta)
	AddContextKnowledgeItems(state, []ContextKnowledgeItem{{
		Source:   "learning_delta",
		Content:  delta.SinceSummary,
		Tags:     []string{"learning", "delta"},
		Priority: 25,
	}})
	return nil
}

func summarizeLearningDelta(interactions []archaeolearning.Interaction, since time.Time) LearningDeltaSummary {
	delta := LearningDeltaSummary{
		ByKind: map[string]int{},
	}
	for _, interaction := range interactions {
		if interaction.Status != archaeolearning.StatusResolved {
			continue
		}
		if !since.IsZero() && !interaction.UpdatedAt.After(since) {
			continue
		}
		delta.TotalResolved++
		switch strings.TrimSpace(string(interaction.SubjectType)) {
		case string(archaeolearning.SubjectPattern):
			switch interactionResolutionKind(interaction) {
			case archaeolearning.ResolutionReject:
				delta.ByKind["pattern_rejected"]++
			default:
				delta.ByKind["pattern_confirmed"]++
				delta.ConfirmedPatterns = append(delta.ConfirmedPatterns, interaction.SubjectID)
			}
		case string(archaeolearning.SubjectTension):
			delta.ByKind["tension_resolved"]++
			delta.ResolvedTensions = append(delta.ResolvedTensions, interaction.SubjectID)
		case string(archaeolearning.SubjectAnchor):
			if interactionResolutionKind(interaction) == archaeolearning.ResolutionRefine {
				delta.ByKind["anchor_refined"]++
				delta.RefinedAnchors = append(delta.RefinedAnchors, interaction.SubjectID)
			}
		}
	}
	delta.ConfirmedPatterns = uniqueStrings(delta.ConfirmedPatterns)
	delta.ResolvedTensions = uniqueStrings(delta.ResolvedTensions)
	delta.RefinedAnchors = uniqueStrings(delta.RefinedAnchors)
	delta.SinceSummary = summarizeLearningDeltaLine(delta)
	return delta
}

func summarizeLearningDeltaLine(delta LearningDeltaSummary) string {
	if delta.TotalResolved == 0 {
		return ""
	}
	parts := make([]string, 0, 3)
	appendCount := func(count int, singular, plural string) {
		if count <= 0 {
			return
		}
		label := singular
		if count != 1 {
			label = plural
		}
		parts = append(parts, fmt.Sprintf("%d %s", count, label))
	}
	appendCount(delta.ByKind["pattern_confirmed"], "pattern confirmed", "patterns confirmed")
	appendCount(delta.ByKind["pattern_rejected"], "pattern rejected", "patterns rejected")
	appendCount(delta.ByKind["tension_resolved"], "tension resolved", "tensions resolved")
	appendCount(delta.ByKind["anchor_refined"], "anchor refined", "anchors refined")
	if len(parts) == 0 {
		appendCount(delta.TotalResolved, "learning interaction resolved", "learning interactions resolved")
	}
	return "Since your last session: " + strings.Join(parts, ", ") + "."
}

func stateTimeValue(state *core.Context, key string) time.Time {
	if state == nil {
		return time.Time{}
	}
	raw, ok := state.Get(key)
	if !ok || raw == nil {
		return time.Time{}
	}
	switch typed := raw.(type) {
	case time.Time:
		return typed.UTC()
	case *time.Time:
		if typed == nil {
			return time.Time{}
		}
		return typed.UTC()
	case string:
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(typed)); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func interactionResolutionKind(interaction archaeolearning.Interaction) archaeolearning.ResolutionKind {
	if interaction.Resolution == nil {
		return ""
	}
	return interaction.Resolution.Kind
}
