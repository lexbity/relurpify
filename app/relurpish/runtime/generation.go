package runtime

import (
	"fmt"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// GenerationState represents the lifecycle state of a generation.
type GenerationState string

const (
	GenerationStateBuilding   GenerationState = "building"
	GenerationStateValidated  GenerationState = "validated"
	GenerationStateQuiescing  GenerationState = "quiescing"
	GenerationStateActive     GenerationState = "active"
	GenerationStateRetiring   GenerationState = "retiring"
	GenerationStateClosed     GenerationState = "closed"
	GenerationStateFailed     GenerationState = "failed"
)

// Generation represents an immutable activated configuration/registry/resource composition.
// It is identified independently of a run and owns its lifecycle state.
type Generation struct {
	// ID is the unique identifier for this generation.
	ID string
	// Workspace is the workspace this generation belongs to.
	Workspace string
	// ConfigSnapshot is the configuration used to build this generation.
	ConfigSnapshot *config.DocumentSnapshot
	// state is the current lifecycle state.
	state GenerationState
	// CreatedAt is when the generation was created.
	CreatedAt time.Time
	// ActivatedAt is when the generation was activated (zero if not yet activated).
	ActivatedAt time.Time
	// RetiredAt is when the generation was retired (zero if not yet retired).
	RetiredAt time.Time
	// activeRunCount is the number of runs currently using this generation.
	activeRunCount int
	// Error contains any error that occurred during generation building/activation.
	Error string
	// ReferenceCount tracks how many components still hold a reference.
	ReferenceCount int

	mu sync.RWMutex
}

// NewGeneration creates a new generation in the building state.
func NewGeneration(id, workspace string, snapshot *config.DocumentSnapshot) *Generation {
	return &Generation{
		ID:             id,
		Workspace:      workspace,
		ConfigSnapshot: snapshot,
		state:          GenerationStateBuilding,
		CreatedAt:      time.Now().UTC(),
		activeRunCount: 0,
		ReferenceCount: 0,
	}
}

// IncrementReference increments the reference count.
func (g *Generation) IncrementReference() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ReferenceCount++
}

// DecrementReference decrements the reference count.
// Returns true if the reference count reaches zero.
func (g *Generation) DecrementReference() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ReferenceCount--
	return g.ReferenceCount == 0
}

// IncrementActiveRun increments the active run count.
func (g *Generation) IncrementActiveRun() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.activeRunCount++
}

// DecrementActiveRun decrements the active run count.
func (g *Generation) DecrementActiveRun() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.activeRunCount--
}

// ActiveRunCount returns the current active run count.
func (g *Generation) ActiveRunCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.activeRunCount
}

// SetState transitions the generation to a new state.
// Returns an error if the transition is invalid.
func (g *Generation) SetState(newState GenerationState) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Validate state transition
	if !isValidTransition(g.state, newState) {
		return fmt.Errorf("invalid state transition from %s to %s", g.state, newState)
	}

	g.state = newState

	// Update timestamps for specific transitions
	switch newState {
	case GenerationStateActive:
		g.ActivatedAt = time.Now().UTC()
	case GenerationStateClosed, GenerationStateRetiring:
		g.RetiredAt = time.Now().UTC()
	}

	return nil
}

// State returns the current state.
func (g *Generation) State() GenerationState {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.state
}

// IsActive returns true if the generation is in an active state.
func (g *Generation) IsActive() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.state == GenerationStateActive
}

// isValidTransition checks if a state transition is valid.
func isValidTransition(from, to GenerationState) bool {
	switch from {
	case GenerationStateBuilding:
		return to == GenerationStateValidated || to == GenerationStateFailed
	case GenerationStateValidated:
		return to == GenerationStateQuiescing || to == GenerationStateFailed
	case GenerationStateQuiescing:
		return to == GenerationStateActive || to == GenerationStateFailed
	case GenerationStateActive:
		return to == GenerationStateRetiring || to == GenerationStateFailed
	case GenerationStateRetiring:
		return to == GenerationStateClosed
	case GenerationStateClosed, GenerationStateFailed:
		// Terminal states - no transitions out
		return false
	default:
		return false
	}
}

// GenerationManager manages multiple generations for a workspace.
type GenerationManager struct {
	workspace string
	generations map[string]*Generation
	active     *Generation
	mu         sync.RWMutex
}

// NewGenerationManager creates a new generation manager.
func NewGenerationManager(workspace string) *GenerationManager {
	return &GenerationManager{
		workspace:   workspace,
		generations: make(map[string]*Generation),
	}
}

// CreateGeneration creates a new generation from a document snapshot.
func (m *GenerationManager) CreateGeneration(snapshot *config.DocumentSnapshot) *Generation {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := fmt.Sprintf("gen_%d", time.Now().UnixNano())
	gen := NewGeneration(id, m.workspace, snapshot)
	m.generations[id] = gen
	return gen
}

// CreateGenerationFromConfig creates a new generation from a workspace config.
func (m *GenerationManager) CreateGenerationFromConfig(cfg *config.WorkspaceConfig) *Generation {
	// For P05, we use a nil snapshot
	return m.CreateGeneration(nil)
}

// GetGeneration returns a generation by ID.
func (m *GenerationManager) GetGeneration(id string) *Generation {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.generations[id]
}

// SetActive sets the active generation.
// This should only be called after quiescence is achieved.
func (m *GenerationManager) SetActive(gen *Generation) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if gen.Workspace != m.workspace {
		return fmt.Errorf("generation belongs to different workspace")
	}

	// Check if there's an active generation that still has active runs
	if m.active != nil && m.active.activeRunCount > 0 {
		return fmt.Errorf("cannot activate new generation while old generation has active runs")
	}

	m.active = gen
	return nil
}

// ActiveGeneration returns the currently active generation.
func (m *GenerationManager) ActiveGeneration() *Generation {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// RetireActive retires the currently active generation.
// Returns an error if there are still active runs.
func (m *GenerationManager) RetireActive() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return fmt.Errorf("no active generation to retire")
	}

	if m.active.activeRunCount > 0 {
		return fmt.Errorf("cannot retire generation with active runs")
	}

	if err := m.active.SetState(GenerationStateRetiring); err != nil {
		return err
	}

	// Close resources
	if err := m.closeGenerationResources(m.active); err != nil {
		return err
	}

	// Transition to closed
	if err := m.active.SetState(GenerationStateClosed); err != nil {
		return err
	}

	m.active = nil
	return nil
}

// closeGenerationResources releases generation-scoped resources. The
// generation record itself owns only metadata; model backends, sandbox
// runners, and managed services are owned by the workspace actor and are
// closed through its shutdown path.
func (m *GenerationManager) closeGenerationResources(gen *Generation) error {
	if gen == nil {
		return nil
	}
	return nil
}

// QuiesceGeneration waits for all active runs in a generation to complete.
// Returns a channel that will be closed when quiescence is achieved.
func (m *GenerationManager) QuiesceGeneration(id string) <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	gen := m.generations[id]
	if gen == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	// Check if already quiesced
	if gen.activeRunCount == 0 {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	// Create a channel that will be closed when count reaches zero
	ch := make(chan struct{})
	go func() {
		for {
			m.mu.RLock()
			count := gen.activeRunCount
			m.mu.RUnlock()
			if count == 0 {
				close(ch)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	return ch
}

// DefaultQuiescenceDeadline is the default deadline for generation quiescence.
const DefaultQuiescenceDeadline = 30 * time.Second

// WaitQuiescence waits for quiescence with a deadline.
// Returns true if quiescence was achieved, false if deadline was reached.
func (m *GenerationManager) WaitQuiescence(id string, deadline time.Duration) bool {
	ch := m.QuiesceGeneration(id)
	select {
	case <-ch:
		return true
	case <-time.After(deadline):
		return false
	}
}

// SettingsChangeReview represents a reviewed settings change before activation.
type SettingsChangeReview struct {
	// FilesChanged lists the configuration files that were changed.
	FilesChanged []string
	// OldHashes maps file paths to their previous digests.
	OldHashes map[string]string
	// NewHashes maps file paths to their new digests.
	NewHashes map[string]string
	// ValidationErrors contains any validation errors.
	ValidationErrors []string
	// ReadyForActivation indicates if the changes are ready to be activated.
	ReadyForActivation bool
	// RequiresRestart indicates if activation requires a runtime restart.
	RequiresRestart bool
	// ActivationPlan describes what will happen during activation.
	ActivationPlan string
}

// SettingsCommand represents a command to modify settings.
type SettingsCommand struct {
	// TargetGeneration is the generation to apply changes to.
	TargetGeneration string
	// Files is the set of files to write.
	Files map[string][]byte
	// ExpectedDigests maps file paths to their expected current digests.
	ExpectedDigests map[string]string
	// ValidateOnly indicates if this is a dry-run validation.
	ValidateOnly bool
}

// SettingsValidation represents the result of validating settings.
type SettingsValidation struct {
	// Valid indicates if the settings are valid.
	Valid bool
	// Errors contains validation errors.
	Errors []string
	// Warnings contains validation warnings.
	Warnings []string
	// EffectiveValues describes the effective configuration after applying the settings.
	EffectiveValues map[string]any
}

// ActivationCommand represents a command to activate staged settings.
type ActivationCommand struct {
	// StagedGeneration is the generation to activate.
	StagedGeneration string
	// Force indicates if activation should proceed even if there are active runs.
	Force bool
	// Deadline is the maximum time to wait for quiescence.
	Deadline time.Duration
}
