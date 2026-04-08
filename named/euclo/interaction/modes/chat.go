package modes

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/ayenitd"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/lexcodex/relurpify/named/euclo/runtime/pretask"
)

// ContextEnrichmentPipeline is the narrow interface the phase needs.
type ContextEnrichmentPipeline interface {
	Run(ctx context.Context, input pretask.PipelineInput) (pretask.EnrichedContextBundle, error)
}

// FileResolverInterface is the narrow interface ContextProposalPhase needs for
// file resolution. *pretask.FileResolver satisfies this interface.
type FileResolverInterface interface {
	Resolve(selections []string, text string) pretask.ResolvedFiles
}

// ContextProposalPhase emits a ContextProposalFrame and awaits the user's
// response before the main execution phase runs.
type ContextProposalPhase struct {
	Pipeline              ContextEnrichmentPipeline
	FileResolver          FileResolverInterface
	ShowConfirmationFrame bool
	Memory                memory.MemoryStore // optional; enables cross-session pin persistence
}

// Execute runs the pipeline, emits the proposal frame, and collects the response.
func (p *ContextProposalPhase) Execute(
	ctx context.Context,
	mc interaction.PhaseMachineContext,
) (interaction.PhaseOutcome, error) {
	state := mc.State
	if state == nil {
		state = make(map[string]any)
	}

	// Get initial query from state
	var userText string
	if queryRaw, ok := state["query"]; ok {
		userText, _ = queryRaw.(string)
	}

	// Get file selections from state
	var userSelections []string
	if selectionsRaw, ok := state["selections"]; ok {
		if selections, ok := selectionsRaw.([]string); ok {
			userSelections = selections
		} else if selectionsSlice, ok := selectionsRaw.([]interface{}); ok {
			for _, item := range selectionsSlice {
				if path, ok := item.(string); ok {
					userSelections = append(userSelections, path)
				}
			}
		}
	}

	// Even with empty query and selections, we still run the pipeline
	// to handle session pins and workflow context retrieval
	if userText == "" && len(userSelections) == 0 {
		// We'll still run the pipeline, but we can skip some processing
		// The pipeline will handle empty input gracefully
	}

	// Resolve current turn files
	resolved := p.FileResolver.Resolve(userSelections, userText)

	// Hydrate session pins: prefer state (set in prior phases this session),
	// fall back to persisted memory for cross-session recall.
	sessionPins := getSessionPins(state)
	if len(sessionPins) == 0 && p.Memory != nil {
		if persisted := loadSessionPinsFromMemory(ctx, p.Memory); len(persisted) > 0 {
			sessionPins = persisted
		}
	}

	// Get workflow ID from state
	workflowID := ""
	if wf, ok := state["euclo.workflow_id"].(string); ok {
		workflowID = wf
	}

	// Run the pipeline if available
	var bundle pretask.EnrichedContextBundle
	var err error

	if p.Pipeline != nil {
		input := pretask.PipelineInput{
			Query:            userText,
			CurrentTurnFiles: resolved.Paths,
			SessionPins:      sessionPins,
			WorkflowID:       workflowID,
		}
		bundle, err = p.Pipeline.Run(ctx, input)
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
	} else {
		// If pipeline is nil, create an empty bundle
		bundle = pretask.EnrichedContextBundle{}
	}

	// Check if we should show confirmation frame
	if !p.ShowConfirmationFrame {
		// Silent mode: skip the frame, load results directly
		confirmedPaths := make([]string, 0)
		for _, file := range bundle.AnchoredFiles {
			confirmedPaths = append(confirmedPaths, file.Path)
		}
		for _, file := range bundle.ExpandedFiles {
			confirmedPaths = append(confirmedPaths, file.Path)
		}

		updatedPins := updateSessionPins(sessionPins, confirmedPaths)
		saveSessionPinsToMemory(ctx, p.Memory, updatedPins)

		stateUpdates := map[string]interface{}{
			"context.confirmed_files": confirmedPaths,
			"context.pinned_files":    updatedPins,
			"context.knowledge_items": append(bundle.KnowledgeTopic, bundle.KnowledgeExpanded...),
			"context.pipeline_trace":  bundle.PipelineTrace,
		}

		return interaction.PhaseOutcome{
			Advance:      true,
			StateUpdates: stateUpdates,
		}, nil
	}

	// Show confirmation frame
	content := convertToContextProposalContent(bundle)

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
			{
				ID:          "add",
				Label:       "Add Files",
				Kind:        interaction.ActionKindSecondary,
				TargetPhase: "context_proposal",
			},
			{
				ID:          "remove",
				Label:       "Remove Files",
				Kind:        interaction.ActionKindSecondary,
				TargetPhase: "context_proposal",
			},
		},
		Continuable: true,
	})

	// Wait for user response
	resp, err := mc.Emitter.AwaitResponse(ctx)
	if err != nil {
		return interaction.PhaseOutcome{
			Advance:      true,
			StateUpdates: map[string]interface{}{},
		}, nil
	}

	// Handle different actions
	switch resp.ActionID {
	case "confirm":
		confirmedPaths := make([]string, 0)
		for _, file := range bundle.AnchoredFiles {
			confirmedPaths = append(confirmedPaths, file.Path)
		}
		for _, file := range bundle.ExpandedFiles {
			confirmedPaths = append(confirmedPaths, file.Path)
		}

		updatedPins := updateSessionPins(sessionPins, confirmedPaths)
		saveSessionPinsToMemory(ctx, p.Memory, updatedPins)

		stateUpdates := map[string]interface{}{
			"context.confirmed_files": confirmedPaths,
			"context.pinned_files":    updatedPins,
			"context.knowledge_items": append(bundle.KnowledgeTopic, bundle.KnowledgeExpanded...),
			"context.pipeline_trace":  bundle.PipelineTrace,
		}

		return interaction.PhaseOutcome{
			Advance:      true,
			StateUpdates: stateUpdates,
		}, nil

	case "skip":
		// Skip enrichment, continue with empty context
		return interaction.PhaseOutcome{
			Advance:      true,
			StateUpdates: map[string]interface{}{},
		}, nil

	case "add":
		// Emit a frame that prompts the TUI to open a file picker
		mc.Emitter.Emit(ctx, interaction.InteractionFrame{
			Kind:  interaction.FrameQuestion,
			Mode:  "chat",
			Phase: "context_proposal",
			Content: interaction.QuestionContent{
				Question:    "Add files to context",
				Description: "Select files to add to the context. You can select multiple files.",
				Options: []interaction.QuestionOption{
					{ID: "file_picker", Label: "Open file picker", Description: "Browse and select files"},
					{ID: "cancel", Label: "Cancel", Description: "Return without adding files"},
				},
			},
			Actions: []interaction.ActionSlot{
				{
					ID:          "select_files",
					Label:       "Select Files",
					Kind:        interaction.ActionKindPrimary,
					TargetPhase: "context_proposal",
				},
				{
					ID:          "cancel_add",
					Label:       "Cancel",
					Kind:        interaction.ActionKindSecondary,
					TargetPhase: "context_proposal",
				},
			},
		})

		// Wait for user response
		resp, err := mc.Emitter.AwaitResponse(ctx)
		if err != nil {
			return interaction.PhaseOutcome{
				Advance:      true,
				StateUpdates: map[string]interface{}{},
			}, nil
		}

		if resp.ActionID == "select_files" {
			// The TUI should have provided selected files in resp.Selections
			if len(resp.Selections) > 0 {
				// Add selected files to user selections in state
				currentSelections := getSessionPins(state)
				newSelections := updateSessionPins(currentSelections, resp.Selections)

				// Update state with new selections
				stateUpdates := map[string]interface{}{
					"selections": newSelections,
				}

				// Re-run the phase with updated selections
				return interaction.PhaseOutcome{
					Advance:      false, // Stay in same phase to re-run with new selections
					StateUpdates: stateUpdates,
				}, nil
			}
		}

		// If cancel or no selections, continue with current state
		return interaction.PhaseOutcome{
			Advance:      false,
			StateUpdates: map[string]interface{}{},
		}, nil

	case "remove":
		// Show current files for removal selection
		// Build a list of all files in the bundle
		allFiles := make([]string, 0)
		for _, file := range bundle.AnchoredFiles {
			allFiles = append(allFiles, file.Path)
		}
		for _, file := range bundle.ExpandedFiles {
			allFiles = append(allFiles, file.Path)
		}

		// Emit a frame showing files that can be removed
		mc.Emitter.Emit(ctx, interaction.InteractionFrame{
			Kind:  interaction.FrameCandidates,
			Mode:  "chat",
			Phase: "context_proposal",
			Content: interaction.CandidatesContent{
				Candidates: func() []interaction.Candidate {
					cands := make([]interaction.Candidate, len(allFiles))
					for i, path := range allFiles {
						cands[i] = interaction.Candidate{
							ID:         path,
							Summary:    path,
							Properties: map[string]string{"type": "file"},
						}
					}
					return cands
				}(),
			},
			Actions: []interaction.ActionSlot{
				{
					ID:          "remove_selected",
					Label:       "Remove Selected",
					Kind:        interaction.ActionKindPrimary,
					TargetPhase: "context_proposal",
				},
				{
					ID:          "cancel",
					Label:       "Cancel",
					Kind:        interaction.ActionKindSecondary,
					TargetPhase: "context_proposal",
				},
			},
		})

		// Wait for user response
		resp, err := mc.Emitter.AwaitResponse(ctx)
		if err != nil {
			return interaction.PhaseOutcome{
				Advance:      true,
				StateUpdates: map[string]interface{}{},
			}, nil
		}

		if resp.ActionID == "remove_selected" && len(resp.Selections) > 0 {
			// Remove selected files from confirmed paths
			confirmedPaths := make([]string, 0)
			selectedSet := make(map[string]bool)
			for _, sel := range resp.Selections {
				selectedSet[sel] = true
			}

			// Keep only files not selected for removal
			for _, file := range bundle.AnchoredFiles {
				if !selectedSet[file.Path] {
					confirmedPaths = append(confirmedPaths, file.Path)
				}
			}
			for _, file := range bundle.ExpandedFiles {
				if !selectedSet[file.Path] {
					confirmedPaths = append(confirmedPaths, file.Path)
				}
			}

			updatedPins := updateSessionPins(sessionPins, confirmedPaths)
			saveSessionPinsToMemory(ctx, p.Memory, updatedPins)

			stateUpdates := map[string]interface{}{
				"context.confirmed_files": confirmedPaths,
				"context.pinned_files":    updatedPins,
				"context.knowledge_items": append(bundle.KnowledgeTopic, bundle.KnowledgeExpanded...),
				"context.pipeline_trace":  bundle.PipelineTrace,
			}

			return interaction.PhaseOutcome{
				Advance:      true,
				StateUpdates: stateUpdates,
			}, nil
		}

		// If cancel or no selections, just continue with current files
		confirmedPaths := make([]string, 0)
		for _, file := range bundle.AnchoredFiles {
			confirmedPaths = append(confirmedPaths, file.Path)
		}
		for _, file := range bundle.ExpandedFiles {
			confirmedPaths = append(confirmedPaths, file.Path)
		}

		updatedPins := updateSessionPins(sessionPins, confirmedPaths)
		saveSessionPinsToMemory(ctx, p.Memory, updatedPins)

		stateUpdates := map[string]interface{}{
			"context.confirmed_files": confirmedPaths,
			"context.pinned_files":    updatedPins,
			"context.knowledge_items": append(bundle.KnowledgeTopic, bundle.KnowledgeExpanded...),
			"context.pipeline_trace":  bundle.PipelineTrace,
		}

		return interaction.PhaseOutcome{
			Advance:      true,
			StateUpdates: stateUpdates,
		}, nil

	default:
		// Default to confirm
		confirmedPaths := make([]string, 0)
		for _, file := range bundle.AnchoredFiles {
			confirmedPaths = append(confirmedPaths, file.Path)
		}
		for _, file := range bundle.ExpandedFiles {
			confirmedPaths = append(confirmedPaths, file.Path)
		}

		updatedPins := updateSessionPins(sessionPins, confirmedPaths)
		saveSessionPinsToMemory(ctx, p.Memory, updatedPins)

		stateUpdates := map[string]interface{}{
			"context.confirmed_files": confirmedPaths,
			"context.pinned_files":    updatedPins,
			"context.knowledge_items": append(bundle.KnowledgeTopic, bundle.KnowledgeExpanded...),
			"context.pipeline_trace":  bundle.PipelineTrace,
		}

		return interaction.PhaseOutcome{
			Advance:      true,
			StateUpdates: stateUpdates,
		}, nil
	}
}

// Helper functions
func getSessionPins(state map[string]any) []string {
	var sessionPins []string
	if pinsRaw, ok := state["context.pinned_files"]; ok {
		if pins, ok := pinsRaw.([]string); ok {
			sessionPins = pins
		} else if pinsSlice, ok := pinsRaw.([]interface{}); ok {
			for _, item := range pinsSlice {
				if path, ok := item.(string); ok {
					sessionPins = append(sessionPins, path)
				}
			}
		}
	}
	return sessionPins
}

func updateSessionPins(existing []string, newPaths []string) []string {
	seen := make(map[string]bool)
	for _, path := range existing {
		seen[path] = true
	}

	result := make([]string, len(existing))
	copy(result, existing)

	for _, path := range newPaths {
		if !seen[path] {
			result = append(result, path)
			seen[path] = true
		}
	}

	return result
}

// loadSessionPinsFromMemory loads session pins from persistent memory.
// Returns nil if memory is unavailable or no pins have been stored yet.
func loadSessionPinsFromMemory(ctx context.Context, mem memory.MemoryStore) []string {
	if mem == nil {
		return nil
	}
	record, ok, err := mem.Recall(ctx, "context.session_pins", memory.MemoryScopeProject)
	if err != nil || !ok || record == nil {
		return nil
	}
	raw, exists := record.Value["paths"]
	if !exists {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// saveSessionPinsToMemory persists session pins for cross-session recall.
func saveSessionPinsToMemory(ctx context.Context, mem memory.MemoryStore, pins []string) {
	if mem == nil || len(pins) == 0 {
		return
	}
	pathsRaw := make([]interface{}, len(pins))
	for i, p := range pins {
		pathsRaw[i] = p
	}
	_ = mem.Remember(ctx, "context.session_pins", map[string]interface{}{"paths": pathsRaw}, memory.MemoryScopeProject)
}

// trustClassToInsertionAction derives the insertion action from a TrustClass string.
// Matches the logic in core.DefaultInsertionDecision without importing framework/core.
func trustClassToInsertionAction(tc string) string {
	switch tc {
	case "builtin-trusted", "workspace-trusted":
		return "direct"
	case "remote-approved":
		return "summarized"
	default:
		if tc == "" {
			return "direct" // unclassified internal content — allow direct insertion
		}
		return "metadata-only"
	}
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
			Path:            item.Path,
			Summary:         item.Summary,
			Score:           item.Score,
			Source:          string(item.Source),
			InsertionAction: trustClassToInsertionAction(item.TrustClass),
		})
	}

	// Convert expanded files
	for _, item := range bundle.ExpandedFiles {
		content.ExpandedFiles = append(content.ExpandedFiles, interaction.ContextFileEntry{
			Path:            item.Path,
			Summary:         item.Summary,
			Score:           item.Score,
			Source:          string(item.Source),
			InsertionAction: trustClassToInsertionAction(item.TrustClass),
		})
	}

	// Convert knowledge items
	for _, item := range bundle.KnowledgeTopic {
		content.KnowledgeItems = append(content.KnowledgeItems, interaction.ContextKnowledgeEntry{
			RefID:           item.RefID,
			Kind:            string(item.Kind),
			Title:           item.Title,
			Summary:         item.Summary,
			Source:          string(item.Source),
			InsertionAction: trustClassToInsertionAction(item.TrustClass),
		})
	}
	for _, item := range bundle.KnowledgeExpanded {
		content.KnowledgeItems = append(content.KnowledgeItems, interaction.ContextKnowledgeEntry{
			RefID:           item.RefID,
			Kind:            string(item.Kind),
			Title:           item.Title,
			Summary:         item.Summary,
			Source:          string(item.Source),
			InsertionAction: trustClassToInsertionAction(item.TrustClass),
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
	fileResolver FileResolverInterface,
	showConfirmationFrame bool,
	mem memory.MemoryStore,
) *interaction.PhaseMachine {
	RegisterChatTriggers(resolver)

	phases := []interaction.PhaseDefinition{}

	// Add context proposal phase if pipeline is provided
	if pipeline != nil && fileResolver != nil {
		phases = append(phases, interaction.PhaseDefinition{
			ID:    "context_proposal",
			Label: "Context",
			Handler: &ContextProposalPhase{
				Pipeline:              pipeline,
				FileResolver:          fileResolver,
				ShowConfirmationFrame: showConfirmationFrame,
				Memory:                mem,
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
// using services from the workspace environment.
func ChatModeWithContext(
	emitter interaction.FrameEmitter,
	resolver *interaction.AgencyResolver,
	workspaceEnv ayenitd.WorkspaceEnvironment,
) *interaction.PhaseMachine {
	return ChatModeLegacy(emitter, resolver)
}

// ChatModeLegacy provides backward compatibility for callers that don't provide pipeline.
func ChatModeLegacy(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
	return ChatMode(emitter, resolver, nil, nil, true, nil)
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
// For backward compatibility with existing tests, returns 3 phases.
// In practice, when pipeline and fileResolver are provided, an additional
// "context_proposal" phase is added by ChatMode().
func ChatPhaseIDs() []string {
	return []string{"intent", "present", "reflect"}
}

// ChatPhaseLabels returns phase labels for the help surface.
func ChatPhaseLabels() []interaction.PhaseInfo {
	ids := ChatPhaseIDs()
	labels := []string{"Intent", "Present", "Reflect"}
	out := make([]interaction.PhaseInfo, len(ids))
	for i := range ids {
		out[i] = interaction.PhaseInfo{ID: ids[i], Label: labels[i]}
	}
	return out
}
