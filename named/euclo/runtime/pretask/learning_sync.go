package pretask

import (
	"context"
	"fmt"
	"strings"

	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	"github.com/lexcodex/relurpify/framework/core"
)

// WorkflowIDResolver extracts the active workflow ID from state.
type WorkflowIDResolver func(state *core.Context) string

// LearningSyncStep refreshes learning interactions for the active workflow and
// injects them into the pretask knowledge pool.
type LearningSyncStep struct {
	LearningService  archaeolearning.Service
	WorkflowResolver WorkflowIDResolver
}

func (s LearningSyncStep) ID() string {
	return "euclo:learning.sync"
}

func (s LearningSyncStep) Run(ctx context.Context, state *core.Context) error {
	if state == nil || s.WorkflowResolver == nil {
		return nil
	}
	workflowID := strings.TrimSpace(s.WorkflowResolver(state))
	if workflowID == "" {
		return nil
	}
	explorationID := firstNonEmpty(
		strings.TrimSpace(state.GetString("euclo.active_exploration_id")),
		strings.TrimSpace(state.GetString("exploration_id")),
	)
	snapshotID := firstNonEmpty(
		strings.TrimSpace(state.GetString("euclo.active_exploration_snapshot_id")),
		strings.TrimSpace(state.GetString("snapshot_id")),
	)
	corpusScope := firstNonEmpty(
		strings.TrimSpace(state.GetString("corpus_scope")),
		strings.TrimSpace(state.GetString("euclo.corpus_scope")),
	)
	codeRevision := firstNonEmpty(
		strings.TrimSpace(state.GetString("euclo.code_revision")),
		strings.TrimSpace(state.GetString("code_revision")),
	)
	pending, blocking, err := s.LearningService.SyncAll(ctx, workflowID, explorationID, snapshotID, corpusScope, codeRevision)
	if err != nil {
		return err
	}
	mergedPending := appendPendingLearningInteractions(state, pending)
	state.Set("euclo.has_blocking_learning", len(blocking) > 0)
	state.Set("euclo.pending_learning_ids", learningInteractionIDs(mergedPending))
	state.Set("euclo.pending_learning_interactions", mergedPending)
	AddContextKnowledgeItems(state, learningInteractionsToKnowledgeItems(mergedPending))
	if len(blocking) > 0 {
		AddBlockingLearningWarning(state, len(blocking))
	}
	if sessionStartTime, ok := state.Get("euclo.session_start_time"); ok && sessionStartTime != nil {
		state.Set("euclo.last_session_time", sessionStartTime)
	}
	return nil
}

func appendPendingLearningInteractions(state *core.Context, pending []archaeolearning.Interaction) []archaeolearning.Interaction {
	if state == nil {
		return nil
	}
	if len(pending) == 0 {
		if _, ok := state.Get("euclo.pending_learning_interactions"); !ok {
			state.Set("euclo.pending_learning_interactions", []archaeolearning.Interaction{})
		}
		if raw, ok := state.Get("euclo.pending_learning_interactions"); ok && raw != nil {
			switch typed := raw.(type) {
			case []archaeolearning.Interaction:
				return append([]archaeolearning.Interaction(nil), typed...)
			case []any:
				out := make([]archaeolearning.Interaction, 0, len(typed))
				for _, item := range typed {
					if interaction, ok := item.(archaeolearning.Interaction); ok {
						out = append(out, interaction)
					}
				}
				return out
			}
		}
		return nil
	}
	existing := make([]archaeolearning.Interaction, 0, len(pending))
	if raw, ok := state.Get("euclo.pending_learning_interactions"); ok && raw != nil {
		switch typed := raw.(type) {
		case []archaeolearning.Interaction:
			existing = append(existing, typed...)
		case []any:
			for _, item := range typed {
				if interaction, ok := item.(archaeolearning.Interaction); ok {
					existing = append(existing, interaction)
				}
			}
		}
	}
	seen := make(map[string]struct{}, len(existing))
	for _, interaction := range existing {
		if id := strings.TrimSpace(interaction.ID); id != "" {
			seen[id] = struct{}{}
		}
	}
	for _, interaction := range pending {
		if id := strings.TrimSpace(interaction.ID); id != "" {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
		}
		existing = append(existing, interaction)
	}
	state.Set("euclo.pending_learning_interactions", existing)
	return append([]archaeolearning.Interaction(nil), existing...)
}

func learningInteractionsToKnowledgeItems(pending []archaeolearning.Interaction) []ContextKnowledgeItem {
	if len(pending) == 0 {
		return nil
	}
	items := make([]ContextKnowledgeItem, 0, len(pending))
	for _, interaction := range pending {
		items = append(items, learningInteractionToKnowledgeItem(interaction))
	}
	return items
}

func learningInteractionIDs(pending []archaeolearning.Interaction) []string {
	if len(pending) == 0 {
		return nil
	}
	ids := make([]string, 0, len(pending))
	for _, interaction := range pending {
		if id := strings.TrimSpace(interaction.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return uniqueStrings(ids)
}

func learningInteractionToKnowledgeItem(interaction archaeolearning.Interaction) ContextKnowledgeItem {
	content := fmt.Sprintf("[Pending: %s] %s - %s", strings.TrimSpace(string(interaction.Kind)), strings.TrimSpace(interaction.Title), strings.TrimSpace(interaction.Description))
	if strings.TrimSpace(content) == "" {
		content = "[Pending learning interaction]"
	}
	return ContextKnowledgeItem{
		Source:  "learning_interaction",
		Content: content,
		Tags: []string{
			strings.TrimSpace(string(interaction.Kind)),
			strings.TrimSpace(string(interaction.SubjectType)),
		},
		Priority: learningPriority(interaction),
	}
}

func learningPriority(interaction archaeolearning.Interaction) int {
	if interaction.Blocking {
		return 100
	}
	return 10
}

// AddBlockingLearningWarning writes a warning context item when blocking
// learning interactions are pending.
func AddBlockingLearningWarning(state *core.Context, count int) {
	if state == nil || count <= 0 {
		return
	}
	state.Set("euclo.blocking_learning_warning", map[string]any{
		"count":   count,
		"message": fmt.Sprintf("%d blocking learning interaction(s) require review", count),
	})
	AddContextKnowledgeItems(state, []KnowledgeEvidenceItem{{
		RefID:      fmt.Sprintf("blocking-learning-%d", count),
		Kind:       KnowledgeKindInteraction,
		Title:      "Blocking learning interactions pending",
		Summary:    fmt.Sprintf("%d blocking learning interaction(s) require review before continuing.", count),
		Source:     EvidenceSourceArchaeoTopic,
		TrustClass: "workspace-trusted",
	}})
}
