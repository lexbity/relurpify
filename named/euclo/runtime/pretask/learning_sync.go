package pretask

import (
	"context"
	"fmt"
	"strings"

	archaeolearning "github.com/lexcodex/relurpify/archaeo/learning"
	"github.com/lexcodex/relurpify/framework/core"
	euclostate "github.com/lexcodex/relurpify/named/euclo/runtime/state"
	"github.com/lexcodex/relurpify/named/euclo/runtime/statebus"
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
	explorationID, _ := euclostate.GetActiveExplorationID(state)
	explorationID = firstNonEmpty(strings.TrimSpace(explorationID), strings.TrimSpace(statebus.GetString(state, "exploration_id")))
	snapshotID, _ := euclostate.GetActiveExplorationSnapshotID(state)
	snapshotID = firstNonEmpty(strings.TrimSpace(snapshotID), strings.TrimSpace(statebus.GetString(state, "snapshot_id")))
	corpusScope, _ := euclostate.GetCorpusScope(state)
	corpusScope = firstNonEmpty(strings.TrimSpace(corpusScope), strings.TrimSpace(statebus.GetString(state, "corpus_scope")))
	codeRevision, _ := euclostate.GetCodeRevision(state)
	codeRevision = firstNonEmpty(strings.TrimSpace(codeRevision), strings.TrimSpace(statebus.GetString(state, "code_revision")))
	pending, blocking, err := s.LearningService.SyncAll(ctx, workflowID, explorationID, snapshotID, corpusScope, codeRevision)
	if err != nil {
		return err
	}
	mergedPending := appendPendingLearningInteractions(state, pending)
	euclostate.SetHasBlockingLearning(state, len(blocking) > 0)
	euclostate.SetPendingLearningIDs(state, learningInteractionIDs(mergedPending))
	euclostate.SetPendingLearningInteractions(state, mergedPending)
	AddContextKnowledgeItems(state, learningInteractionsToKnowledgeItems(mergedPending))
	if len(blocking) > 0 {
		AddBlockingLearningWarning(state, len(blocking))
	}
	if sessionStartTime, ok := euclostate.GetSessionStartTime(state); ok {
		euclostate.SetLastSessionTime(state, sessionStartTime)
	}
	return nil
}

func appendPendingLearningInteractions(state *core.Context, pending []archaeolearning.Interaction) []archaeolearning.Interaction {
	if state == nil {
		return nil
	}
	if len(pending) == 0 {
		if _, ok := euclostate.GetPendingLearningInteractions(state); !ok {
			euclostate.SetPendingLearningInteractions(state, []archaeolearning.Interaction{})
		}
		if raw, ok := euclostate.GetPendingLearningInteractions(state); ok {
			if typed, ok := raw.([]archaeolearning.Interaction); ok {
				return append([]archaeolearning.Interaction(nil), typed...)
			}
			if typed, ok := raw.([]any); ok {
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
	if raw, ok := euclostate.GetPendingLearningInteractions(state); ok {
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
	euclostate.SetPendingLearningInteractions(state, existing)
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
	statebus.SetAny(state, "euclo.blocking_learning_warning", map[string]any{
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
