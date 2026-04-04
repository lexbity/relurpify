package modes

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/ayenitd"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/lexcodex/relurpify/named/euclo/runtime/pretask"
)

// ContextEnrichmentPipeline is the narrow interface the phase needs.
type ContextEnrichmentPipeline interface {
	Run(ctx context.Context, input pretask.PipelineInput) (pretask.EnrichedContextBundle, error)
}

// ContextProposalPhase emits a ContextProposalFrame and awaits the user's
// response before the main execution phase runs.
type ContextProposalPhase struct {
	Pipeline     ContextEnrichmentPipeline
	FileResolver *pretask.FileResolver
}

// Execute runs the pipeline, emits the proposal frame, and collects the response.
func (p *ContextProposalPhase) Execute(
	ctx context.Context,
	mc interaction.PhaseMachineContext,
) (interaction.PhaseOutcome, error) {
	// Get user response from the context
	userResp := mc.UserResponse
	if userResp == nil {
		// No user response yet, emit a status frame
		mc.Emitter.Emit(ctx, interaction.InteractionFrame{
			Kind:    interaction.FrameStatus,
			Mode:    "chat",
			Phase:   "context_proposal",
			Content: interaction.StatusContent{Message: "Preparing context enrichment..."},
		})
		return interaction.PhaseOutcome{
			Advance:      false,
			StateUpdates: map[string]interface{}{},
		}, nil
	}

	// Resolve current turn files from user response
	resolved := p.FileResolver.Resolve(userResp.Selections, userResp.Text)
	
	// Load session pins from memory
	var sessionPins []string
	// TODO: Load from HybridMemory["context.pinned_files"] (MemoryScopeSession)
	// For now, use empty
	
	// Get workflow ID from state
	workflowID := ""
	if state := mc.State; state != nil {
		if wf, ok := state["euclo.workflow_id"].(string); ok {
			workflowID = wf
		}
	}

	// Run the pipeline
	input := pretask.PipelineInput{
		Query:            userResp.Text,
		CurrentTurnFiles: resolved.Paths,
		SessionPins:      sessionPins,
		WorkflowID:       workflowID,
	}

	bundle, err := p.Pipeline.Run(ctx, input)
	if err != nil {
		// Log error but continue
		mc.Emitter.Emit(ctx, interaction.InteractionFrame{
			Kind:    interaction.FrameStatus,
			Mode:    "chat",
			Phase:   "context_proposal",
			Content: interaction.StatusContent{Message: fmt.Sprintf("Context enrichment had issues: %v", err)},
		})
		// Continue without enrichment
		return interaction.PhaseOutcome{
			Advance:      true,
			StateUpdates: map[string]interface{}{},
		}, nil
	}

	// Convert bundle to ContextProposalContent
	content := convertToContextProposalContent(bundle)

	// Emit the proposal frame
	mc.Emitter.Emit(ctx, interaction.InteractionFrame{
		Kind:    interaction.FrameProposal,
		Mode:    "chat",
		Phase:   "context_proposal",
		Content: content,
		Actions: []interaction.ActionSlot{
			{
				ID:          "confirm",
				Label:       "Confirm",
				Kind:        interaction.ActionKindPrimary,
				Default:     true,
				TargetPhase: "intent",
			},
			{
				ID:          "skip",
				Label:       "Skip",
				Kind:        interaction.ActionKindSecondary,
				TargetPhase: "intent",
			},
		},
		Continuable: true,
	})

	// Wait for user response
	return interaction.PhaseOutcome{
		Advance:      false, // Wait for user to confirm or skip
		StateUpdates: map[string]interface{}{},
	}, nil
}

func convertToContextProposalContent(bundle pretask.EnrichedContextBundle) interaction.ContextProposalContent {
	content := interaction.ContextProposalContent{
		PipelineTrace: interaction.PipelineTrace{
			AnchorsExtracted:      bundle.PipelineTrace.AnchorsExtracted,
			AnchorsConfirmed:      bundle.PipelineTrace.AnchorsConfirmed,
			Stage1CodeResults:     bundle.PipelineTrace.Stage1CodeResults,
			Stage1ArchaeoResults:  bundle.PipelineTrace.Stage1ArchaeoResults,
			HypotheticalGenerated: bundle.PipelineTrace.HypotheticalGenerated,
			HypotheticalTokens:    bundle.PipelineTrace.HypotheticalTokens,
			Stage3ArchaeoResults:  bundle.PipelineTrace.Stage3ArchaeoResults,
			FallbackUsed:          bundle.PipelineTrace.FallbackUsed,
			FallbackReason:        bundle.PipelineTrace.FallbackReason,
			TotalTokenEstimate:    bundle.PipelineTrace.TotalTokenEstimate,
		},
	}

	// Convert anchored files
	for _, item := range bundle.AnchoredFiles {
		content.AnchoredFiles = append(content.AnchoredFiles, interaction.ContextFileEntry{
			Path:    item.Path,
			Summary: item.Summary,
			Score:   item.Score,
			Source:  string(item.Source),
		})
	}

	// Convert expanded files
	for _, item := range bundle.ExpandedFiles {
		content.ExpandedFiles = append(content.ExpandedFiles, interaction.ContextFileEntry{
			Path:    item.Path,
			Summary: item.Summary,
			Score:   item.Score,
			Source:  string(item.Source),
		})
	}

	// Convert knowledge items
	for _, item := range bundle.KnowledgeTopic {
		content.KnowledgeItems = append(content.KnowledgeItems, interaction.ContextKnowledgeEntry{
			RefID:   item.RefID,
			Kind:    string(item.Kind),
			Title:   item.Title,
			Summary: item.Summary,
			Source:  string(item.Source),
		})
	}
	for _, item := range bundle.KnowledgeExpanded {
		content.KnowledgeItems = append(content.KnowledgeItems, interaction.ContextKnowledgeEntry{
			RefID:   item.RefID,
			Kind:    string(item.Kind),
			Title:   item.Title,
			Summary: item.Summary,
			Source:  string(item.Source),
		})
	}

	return content
}

// ChatMode builds the phase machine for the chat interaction mode.
//
// Phases: context_proposal → intent → present → reflect
//
// Chat mode is for conversational tasks: answering questions, explaining
// concepts, and optionally proposing transitions to code/debug for
// implementation or investigation work.
func ChatMode(
	emitter interaction.FrameEmitter,
	resolver *interaction.AgencyResolver,
	pipeline ContextEnrichmentPipeline,
	fileResolver *pretask.FileResolver,
) *interaction.PhaseMachine {
	RegisterChatTriggers(resolver)

	phases := []interaction.PhaseDefinition{}
	
	// Add context proposal phase if pipeline is provided
	if pipeline != nil && fileResolver != nil {
		phases = append(phases, interaction.PhaseDefinition{
			ID:      "context_proposal",
			Label:   "Context",
			Handler: &ContextProposalPhase{
				Pipeline:     pipeline,
				FileResolver: fileResolver,
			},
		})
	}
	
	// Add the original phases
	phases = append(phases, []interaction.PhaseDefinition{
		{
			ID:      "intent",
			Label:   "Intent",
			Handler: &ChatIntentPhase{},
		},
		{
			ID:      "present",
			Label:   "Present",
			Handler: &ChatPresentPhase{},
		},
		{
			ID:        "reflect",
			Label:     "Reflect",
			Handler:   &ChatReflectPhase{},
			Skippable: true,
			SkipWhen:  skipChatReflect,
		},
	}...)

	return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:     "chat",
		Emitter:  emitter,
		Resolver: resolver,
		Phases:   phases,
	})
}

// ChatModeWithContext is a convenience function that creates a chat mode with context enrichment
func ChatModeWithContext(
	emitter interaction.FrameEmitter,
	resolver *interaction.AgencyResolver,
	workspaceEnv ayenitd.WorkspaceEnvironment,
) *interaction.PhaseMachine {
	// For now, just use the legacy chat mode
	// We'll implement proper context enrichment later
	return ChatModeLegacy(emitter, resolver)
}

// ChatModeLegacy provides backward compatibility for callers that don't provide pipeline.
func ChatModeLegacy(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
	return ChatMode(emitter, resolver, nil, nil)
}

// skipChatReflect skips the reflect phase when the user explicitly opts out
// or when the interaction was a simple ask with no follow-on actions.
func skipChatReflect(state map[string]any, _ *interaction.ArtifactBundle) bool {
	if v, _ := state["chat.skip_reflect"].(bool); v {
		return true
	}
	// Skip reflect for pure ask interactions that produced a direct answer.
	if subMode, _ := state["chat.sub_mode"].(string); subMode == "ask" {
		if _, answered := state["present.answered"].(bool); answered {
			return true
		}
	}
	return false
}

// RegisterChatTriggers registers agency triggers for chat mode.
func RegisterChatTriggers(resolver *interaction.AgencyResolver) {
	if resolver == nil {
		return
	}
	resolver.RegisterTrigger("chat", interaction.AgencyTrigger{
		Phrases:     []string{"implement this", "can you write", "write this"},
		Description: "Propose transition to code mode for implementation",
	})
	resolver.RegisterTrigger("chat", interaction.AgencyTrigger{
		Phrases:     []string{"debug this", "why is it failing"},
		Description: "Propose transition to debug mode for investigation",
	})
	resolver.RegisterTrigger("chat", interaction.AgencyTrigger{
		Phrases:     []string{"review this"},
		Description: "Propose transition to review mode",
	})
}

// ChatPhaseIDs returns the ordered phase IDs for chat mode.
func ChatPhaseIDs() []string {
	return []string{"context_proposal", "intent", "present", "reflect"}
}

// ChatPhaseLabels returns phase labels for the help surface.
func ChatPhaseLabels() []interaction.PhaseInfo {
	ids := ChatPhaseIDs()
	labels := []string{"Context", "Intent", "Present", "Reflect"}
	out := make([]interaction.PhaseInfo, len(ids))
	for i := range ids {
		out[i] = interaction.PhaseInfo{ID: ids[i], Label: labels[i]}
	}
	return out
}
