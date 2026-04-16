// Package intake provides the single-pass classification pipeline for Euclo.
// It replaces the double runtimeState() call pattern with an explicit enrichment
// step that produces a fully-resolved ClassifiedEnvelope.
package intake

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	euclostate "github.com/lexcodex/relurpify/named/euclo/runtime/state"
)

// CapabilityClassifier is the interface for capability intent classification.
// It replaces the ad-hoc classifier in runtime/capability_classifier.go.
type CapabilityClassifier interface {
	// Classify returns the recommended capability sequence for the given instruction
	// and mode. It must not write to state.
	// Returns: (sequence, operator, error)
	// operator: "AND" | "OR"
	Classify(ctx context.Context, instruction, modeID string) ([]string, string, error)
}

// ClassifiedEnvelope holds the complete result of the single-pass enrichment pipeline.
// It is produced by RunEnrichment and persisted to state via SeedClassifiedEnvelope.
type ClassifiedEnvelope struct {
	Envelope       eucloruntime.TaskEnvelope
	Classification eucloruntime.TaskClassification
	Mode           euclotypes.ModeResolution
	Profile        euclotypes.ExecutionProfileSelection
	Work           eucloruntime.UnitOfWork
}

// RunEnrichment runs the full single-pass classification pipeline.
// It does not read from or write to state. Callers are responsible for
// persisting the result to state via SeedClassifiedEnvelope.
func RunEnrichment(
	ctx context.Context,
	task *core.Task,
	state *core.Context,
	env agentenv.AgentEnvironment,
	modeRegistry *euclotypes.ModeRegistry,
	profileRegistry *euclotypes.ExecutionProfileRegistry,
	capabilityClassifier CapabilityClassifier,
	semanticInputs eucloruntime.SemanticInputBundle,
	resolvedPolicy eucloruntime.ResolvedExecutionPolicy,
	executorDescriptor eucloruntime.WorkUnitExecutorDescriptor,
) (ClassifiedEnvelope, error) {
	// Step 1: Normalize envelope
	envelope := NormalizeEnvelope(task, state, env.Registry)

	// Step 2: Classify task
	classification := ClassifyTask(envelope)

	// Step 3: Resolve mode
	mode := ResolveMode(envelope, classification, modeRegistry)
	envelope.ResolvedMode = mode.ModeID

	// Step 4: Resolve profile
	profile := ResolveProfile(envelope, classification, mode, profileRegistry)
	envelope.ExecutionProfile = profile.ProfileID

	// Step 5: Enrich with capability intent classification (may call LLM for Tier 2)
	if capabilityClassifier != nil && mode.ModeID != "" {
		seq, op, err := capabilityClassifier.Classify(ctx, envelope.Instruction, mode.ModeID)
		if err == nil && len(seq) > 0 {
			envelope.CapabilitySequence = seq
			envelope.CapabilitySequenceOperator = op
			envelope.CapabilityClassificationSource = "classifier"
			// Note: Meta could be enriched with match details if classifier provides them
		}
		// If classifier fails or returns empty, we continue with envelope defaults
	}

	// Step 6: Build unit of work
	work := BuildUnitOfWork(task, state, envelope, classification, mode, profile, modeRegistry, semanticInputs, resolvedPolicy, executorDescriptor)

	return ClassifiedEnvelope{
		Envelope:       envelope,
		Classification: classification,
		Mode:           mode,
		Profile:        profile,
		Work:           work,
	}, nil
}

// SeedClassifiedEnvelope persists a classified envelope to state using typed accessors.
// This replaces the seedRuntimeState function.
func SeedClassifiedEnvelope(state *core.Context, classified ClassifiedEnvelope) {
	if state == nil {
		return
	}

	// Seed core state
	euclostate.SetEnvelope(state, classified.Envelope)
	euclostate.SetClassification(state, classified.Classification)
	euclostate.SetMode(state, classified.Mode.ModeID)
	euclostate.SetExecutionProfile(state, classified.Profile.ProfileID)
	euclostate.SetUnitOfWork(state, classified.Work)

	// Seed extended state
	euclostate.SetModeResolution(state, classified.Mode)
	euclostate.SetExecutionProfileSelection(state, classified.Profile)
	euclostate.SetSemanticInputs(state, classified.Work.SemanticInputs)
	euclostate.SetResolvedExecutionPolicy(state, classified.Work.ResolvedPolicy)
	euclostate.SetExecutorDescriptor(state, classified.Work.ExecutorDescriptor)

	// Seed capability classification results
	if len(classified.Envelope.CapabilitySequence) > 0 {
		euclostate.SetPreClassifiedCapabilitySequence(state, classified.Envelope.CapabilitySequence)
	}
	if classified.Envelope.CapabilitySequenceOperator != "" {
		euclostate.SetCapabilitySequenceOperator(state, classified.Envelope.CapabilitySequenceOperator)
	}
	if classified.Envelope.CapabilityClassificationSource != "" {
		euclostate.SetClassificationSource(state, classified.Envelope.CapabilityClassificationSource)
	}
	if classified.Envelope.CapabilityClassificationMeta != "" {
		euclostate.SetClassificationMeta(state, classified.Envelope.CapabilityClassificationMeta)
	}

	// Update unit of work history
	history := []eucloruntime.UnitOfWorkHistoryEntry(nil)
	if raw, ok := euclostate.GetUnitOfWorkHistory(state); ok {
		history = append(history, raw...)
	}
	if len(history) == 0 {
		if existing, ok := euclostate.GetUnitOfWork(state); ok && existing.ID != "" {
			history = eucloruntime.UpdateUnitOfWorkHistory(history, existing, existing.UpdatedAt)
		}
	}
	euclostate.SetUnitOfWorkHistory(state, eucloruntime.UpdateUnitOfWorkHistory(history, classified.Work, classified.Work.UpdatedAt))
}

// RebuildUnitOfWork rebuilds only the UnitOfWork after a state change (e.g., restore).
// It preserves the already-classified envelope, mode, and profile.
// This is used when restoreExecutionContinuity changes state and work needs to be refreshed.
func RebuildUnitOfWork(
	task *core.Task,
	state *core.Context,
	classified ClassifiedEnvelope,
	modeRegistry *euclotypes.ModeRegistry,
	semanticInputs eucloruntime.SemanticInputBundle,
	resolvedPolicy eucloruntime.ResolvedExecutionPolicy,
	executorDescriptor eucloruntime.WorkUnitExecutorDescriptor,
) eucloruntime.UnitOfWork {
	// Rebuild work with potentially updated state (e.g., after restore)
	return BuildUnitOfWork(task, state, classified.Envelope, classified.Classification, classified.Mode, classified.Profile, modeRegistry, semanticInputs, resolvedPolicy, executorDescriptor)
}

// capabilityClassifierAdapter adapts the runtime.CapabilityIntentClassifier to the CapabilityClassifier interface.
// This allows the existing classifier to be used with the new intake pipeline.
type capabilityClassifierAdapter struct {
	classifier *eucloruntime.CapabilityIntentClassifier
}

// Classify implements the CapabilityClassifier interface.
func (a *capabilityClassifierAdapter) Classify(ctx context.Context, instruction, modeID string) ([]string, string, error) {
	if a.classifier == nil {
		return nil, "", fmt.Errorf("classifier not available")
	}

	result, err := a.classifier.Classify(ctx, instruction, modeID)
	if err != nil {
		return nil, "", err
	}

	return result.Sequence, result.Operator, nil
}

// NewCapabilityClassifier creates a CapabilityClassifier from a runtime.CapabilityIntentClassifier.
// This is a temporary adapter during the transition period.
func NewCapabilityClassifier(classifier *eucloruntime.CapabilityIntentClassifier) CapabilityClassifier {
	if classifier == nil {
		return nil
	}
	return &capabilityClassifierAdapter{classifier: classifier}
}

// SimpleCapabilityClassifier is a simple implementation of CapabilityClassifier for testing.
type SimpleCapabilityClassifier struct {
	Registry *euclorelurpic.Registry
	Model    core.LanguageModel
	Keywords map[string][]string // capability ID -> extra keywords
}

// Classify implements a simple tier-1 keyword matching classification.
func (s *SimpleCapabilityClassifier) Classify(ctx context.Context, instruction, modeID string) ([]string, string, error) {
	if s.Registry == nil {
		return nil, "", fmt.Errorf("registry not available")
	}

	// Try keyword matching
	matches := s.Registry.MatchByKeywords(instruction, modeID, s.Keywords)
	if len(matches) > 0 {
		// Return the best match
		return []string{matches[0].Descriptor.ID}, "AND", nil
	}

	// Fall back to mode default
	if desc, ok := s.Registry.FallbackCapabilityForMode(modeID); ok {
		return []string{desc.ID}, "AND", nil
	}

	return nil, "", fmt.Errorf("no capability match for mode %q", modeID)
}
