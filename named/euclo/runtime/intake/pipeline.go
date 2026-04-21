// Package intake provides the single-pass classification pipeline for Euclo.
// It replaces the older two-step state seeding path with an explicit
// enrichment step that produces a fully-resolved ClassifiedEnvelope.
package intake

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	euclorelurpic "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	euclokeys "codeburg.org/lexbit/relurpify/named/euclo/runtime/keys"
	"codeburg.org/lexbit/relurpify/named/euclo/runtime/statebus"
)

// CapabilityClassifier is the interface for capability intent classification
// used by the single-pass enrichment pipeline.
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
// This replaces the older seed-and-persist helper.
func SeedClassifiedEnvelope(state *core.Context, classified ClassifiedEnvelope) {
	if state == nil {
		return
	}

	// Seed core state
	statebus.SetAny(state, euclokeys.KeyEnvelope, classified.Envelope)
	statebus.SetAny(state, euclokeys.KeyClassification, classified.Classification)
	statebus.SetAny(state, euclokeys.KeyMode, classified.Mode.ModeID)
	statebus.SetAny(state, euclokeys.KeyExecutionProfile, classified.Profile.ProfileID)
	statebus.SetAny(state, euclokeys.KeyUnitOfWork, classified.Work)

	// Seed extended state
	statebus.SetAny(state, euclokeys.KeyModeResolution, classified.Mode)
	statebus.SetAny(state, euclokeys.KeyExecutionProfileSelection, classified.Profile)
	statebus.SetAny(state, euclokeys.KeySemanticInputs, classified.Work.SemanticInputs)
	statebus.SetAny(state, euclokeys.KeyResolvedExecutionPolicy, classified.Work.ResolvedPolicy)
	statebus.SetAny(state, euclokeys.KeyExecutorDescriptor, classified.Work.ExecutorDescriptor)

	// Seed capability classification results
	if len(classified.Envelope.CapabilitySequence) > 0 {
		statebus.SetAny(state, euclokeys.KeyPreClassifiedCapSeq, classified.Envelope.CapabilitySequence)
	}
	if classified.Envelope.CapabilitySequenceOperator != "" {
		statebus.SetAny(state, euclokeys.KeyCapabilitySequenceOperator, classified.Envelope.CapabilitySequenceOperator)
	}
	if classified.Envelope.CapabilityClassificationSource != "" {
		statebus.SetAny(state, euclokeys.KeyClassificationSource, classified.Envelope.CapabilityClassificationSource)
	}
	if classified.Envelope.CapabilityClassificationMeta != "" {
		statebus.SetAny(state, euclokeys.KeyClassificationMeta, classified.Envelope.CapabilityClassificationMeta)
	}

	// Update unit of work history
	history := []eucloruntime.UnitOfWorkHistoryEntry(nil)
	if raw, ok := statebus.GetAny(state, euclokeys.KeyUnitOfWorkHistory); ok {
		if typed, ok := raw.([]eucloruntime.UnitOfWorkHistoryEntry); ok {
			history = append(history, typed...)
		}
	}
	if len(history) == 0 {
		if existing, ok := statebus.GetAny(state, euclokeys.KeyUnitOfWork); ok {
			if typed, ok := existing.(eucloruntime.UnitOfWork); ok && typed.ID != "" {
				history = eucloruntime.UpdateUnitOfWorkHistory(history, typed, typed.UpdatedAt)
			}
		}
	}
	statebus.SetAny(state, euclokeys.KeyUnitOfWorkHistory, eucloruntime.UpdateUnitOfWorkHistory(history, classified.Work, classified.Work.UpdatedAt))
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
