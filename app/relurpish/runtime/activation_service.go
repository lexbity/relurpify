// Package runtime provides the Relurpish workspace owner and its runtime services.
// This file implements the ActivationService for P05.
package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// ActivationService manages generation activation with quiescence checks and safety.
// It implements the generation activation flow described in P05.
type ActivationService struct {
	mu                sync.RWMutex
	manager           *GenerationManager
	workspaceRoot     string
	fileTxnService    FileTransactionPort
	quiescenceTimeout time.Duration
}

// NewActivationService creates a new activation service.
func NewActivationService(
	manager *GenerationManager,
	workspaceRoot string,
	fileTxnService FileTransactionPort,
) *ActivationService {
	return &ActivationService{
		manager:           manager,
		workspaceRoot:     workspaceRoot,
		fileTxnService:    fileTxnService,
		quiescenceTimeout: 30 * time.Second,
	}
}

// ActivationRequest represents a request to activate a generation.
type ActivationRequest struct {
	StagedGenerationID string
	Force              bool
	Deadline          time.Duration
}

// ActivationResult represents the result of an activation.
type ActivationResult struct {
	Success             bool
	OldGenerationID     string
	NewGenerationID     string
	QuiescenceWaitTime  time.Duration
	Error               error
	RequiresRollback    bool
	RollbackTransaction TransactionID
}

// ActivateGeneration activates a staged generation, waiting for quiescence.
// This implements the activation flow with the quiescence checks from P05.
func (s *ActivationService) ActivateGeneration(ctx context.Context, req ActivationRequest) ActivationResult {
	result := ActivationResult{
		NewGenerationID: req.StagedGenerationID,
	}

	// Get the staged generation
	stagedGen := s.manager.GetGeneration(req.StagedGenerationID)
	if stagedGen == nil {
		result.Error = fmt.Errorf("staged generation %s not found", req.StagedGenerationID)
		return result
	}

	// Check that the staged generation is in the right state
	if stagedGen.State() != GenerationStateValidated {
		result.Error = fmt.Errorf("staged generation must be in Validated state, got %s", stagedGen.State())
		return result
	}

	// Check if there's an active generation
	activeGen := s.manager.ActiveGeneration()
	
	if activeGen != nil {
		// Wait for quiescence of the old generation
		startWait := time.Now()
		
		// If force is false, we need to wait for active runs to complete
		if !req.Force {
			// Check current active run count
			if activeGen.ActiveRunCount() > 0 {
				// Wait for quiescence with timeout
				ch := s.manager.QuiesceGeneration(activeGen.ID)
				
				select {
				case <-ch:
					// Quiescence achieved
					result.QuiescenceWaitTime = time.Since(startWait)
				case <-time.After(s.quiescenceTimeout):
					if req.Deadline > 0 {
						// Use the request deadline
						select {
						case <-ch:
							result.QuiescenceWaitTime = time.Since(startWait)
						case <-time.After(req.Deadline):
							result.Error = fmt.Errorf("quiescence timeout after %v", req.Deadline)
							return result
						}
					} else {
						result.Error = fmt.Errorf("quiescence timeout after %v", s.quiescenceTimeout)
						return result
					}
				case <-ctx.Done():
					result.Error = ctx.Err()
					return result
				}
			}
		} else {
			// Force mode: cancel active runs
			// This would integrate with the run coordinator in a full implementation
			// For P05, we just proceed with activation
			result.QuiescenceWaitTime = time.Since(startWait)
		}
	}

	// Now perform the activation
	oldGenID := ""
	if activeGen != nil {
		oldGenID = activeGen.ID
		
		// Transition old generation to retiring state
		if err := activeGen.SetState(GenerationStateRetiring); err != nil {
			result.Error = fmt.Errorf("failed to retire old generation: %w", err)
			return result
		}
	}

	// Transition staged generation through states
	if err := stagedGen.SetState(GenerationStateQuiescing); err != nil {
		result.Error = fmt.Errorf("failed to transition to quiescing: %w", err)
		// Attempt rollback
		if oldGenID != "" {
			oldGen := s.manager.GetGeneration(oldGenID)
			if oldGen != nil {
				_ = oldGen.SetState(GenerationStateActive)
			}
		}
		return result
	}

	if err := stagedGen.SetState(GenerationStateActive); err != nil {
		result.Error = fmt.Errorf("failed to transition to active: %w", err)
		// Attempt rollback
		if oldGenID != "" {
			oldGen := s.manager.GetGeneration(oldGenID)
			if oldGen != nil {
				_ = oldGen.SetState(GenerationStateActive)
			}
		}
		_ = stagedGen.SetState(GenerationStateValidated)
		return result
	}

	// Set the new generation as active
	if err := s.manager.SetActive(stagedGen); err != nil {
		result.Error = fmt.Errorf("failed to set active generation: %w", err)
		// Attempt rollback
		_ = stagedGen.SetState(GenerationStateValidated)
		if oldGenID != "" {
			oldGen := s.manager.GetGeneration(oldGenID)
			if oldGen != nil {
				_ = oldGen.SetState(GenerationStateActive)
				_ = s.manager.SetActive(oldGen)
			}
		}
		return result
	}

	// Success!
	result.Success = true
	result.OldGenerationID = oldGenID
	result.NewGenerationID = stagedGen.ID

	// Retire old generation in background
	if oldGenID != "" {
		go s.retireOldGeneration(oldGenID)
	}

	return result
}

// retireOldGeneration transitions an old generation to closed state.
func (s *ActivationService) retireOldGeneration(oldGenID string) {
	// Wait a bit to ensure the new generation is stable
	time.Sleep(100 * time.Millisecond)
	
	oldGen := s.manager.GetGeneration(oldGenID)
	if oldGen == nil {
		return
	}
	
	// Transition to closed state
	_ = oldGen.SetState(GenerationStateClosed)
}

// CanActivateGeneration checks if a generation can be activated.
func (s *ActivationService) CanActivateGeneration(generationID string) (bool, error) {
	gen := s.manager.GetGeneration(generationID)
	if gen == nil {
		return false, fmt.Errorf("generation %s not found", generationID)
	}

	// Check state
	if gen.State() != GenerationStateValidated {
		return false, fmt.Errorf("generation must be in Validated state")
	}

	// Check if there's an active generation with active runs
	activeGen := s.manager.ActiveGeneration()
	if activeGen != nil {
		if activeGen.ActiveRunCount() > 0 {
			return false, fmt.Errorf("active generation has %d active runs", activeGen.ActiveRunCount())
		}
	}

	return true, nil
}

// GetActivationPlan returns the activation plan for a generation.
func (s *ActivationService) GetActivationPlan(generationID string) (*ActivationPlan, error) {
	gen := s.manager.GetGeneration(generationID)
	if gen == nil {
		return nil, fmt.Errorf("generation %s not found", generationID)
	}

	plan := &ActivationPlan{
		TargetGeneration: generationID,
		CurrentGeneration: "",
		RequiresQuiescence: false,
		QuiescenceTimeout:  s.quiescenceTimeout,
		Steps: []string{
			"Validate staged generation",
			"Check quiescence of current active generation",
			"Transition staged generation to quiescing",
			"Transition staged generation to active",
			"Set as active generation",
			"Retire old generation",
		},
	}

	activeGen := s.manager.ActiveGeneration()
	if activeGen != nil {
		plan.CurrentGeneration = activeGen.ID
		if activeGen.ActiveRunCount() > 0 {
			plan.RequiresQuiescence = true
			plan.QuiescenceMessage = fmt.Sprintf("Waiting for %d active runs to complete", activeGen.ActiveRunCount())
		}
	}

	return plan, nil
}

// ActivationPlan represents the plan for activating a generation.
type ActivationPlan struct {
	TargetGeneration  string
	CurrentGeneration string
	RequiresQuiescence bool
	QuiescenceTimeout  time.Duration
	QuiescenceMessage  string
	Steps              []string
}

// ActivationPort defines the interface for activation operations.
// This is used for dependency injection and testing.
type ActivationPort interface {
	ActivateGeneration(ctx context.Context, req ActivationRequest) ActivationResult
	CanActivateGeneration(generationID string) (bool, error)
	GetActivationPlan(generationID string) (*ActivationPlan, error)
}

// Ensure ActivationService implements ActivationPort.
var _ ActivationPort = (*ActivationService)(nil)

// SaveAndStageGeneration saves config files and stages a new generation.
// This combines file transaction with generation lifecycle management.
func (s *ActivationService) SaveAndStageGeneration(
	ctx context.Context,
	configFiles map[string][]byte,
	oldDigests map[string]string,
	workspace string,
) (*config.DocumentSnapshot, TransactionID, error) {
	// Generate transaction ID
	txnID := TransactionID(fmt.Sprintf("gen_staging_%d", time.Now().UnixNano()))

	// Execute transaction
	// Create and execute the file transaction directly
	// Build changes from configFiles
	var changes []FileChange
	for path, content := range configFiles {
		changes = append(changes, FileChange{
			Path:     path,
			Content:  content,
			OldDigest: oldDigests[path],
		})
	}
	
	result := s.fileTxnService.ExecuteTransaction(FileTransactionRequest{
		TransactionID: txnID,
		Changes:       changes,
		BackupEnabled: true,
		Description:   "Generation staging",
	})

	if !result.Success {
		return nil, "", fmt.Errorf("execute transaction: %v", result.Error)
	}

	// Create a document snapshot for the new generation
	// In a real implementation, this would be more complete
	snapshot := &config.DocumentSnapshot{
		Document: &config.Document{
			APIVersion: "v1",
			Kind:       "Workspace",
			Metadata: config.DocumentMetadata{
				Name: workspace,
			},
		},
		SourcePath: workspace + ".yaml",
		LoadedAt:   time.Now().UTC(),
	}

	// Create new generation
	gen := s.manager.CreateGeneration(snapshot)
	
	// Set to validated state
	if err := gen.SetState(GenerationStateValidated); err != nil {
		// Rollback
		_ = s.fileTxnService.RollbackTransaction(txnID)
		return nil, "", fmt.Errorf("set generation state: %w", err)
	}

	return snapshot, txnID, nil
}

// EmergencyRollback performs an emergency rollback of a failed activation.
func (s *ActivationService) EmergencyRollback(generationID string) error {
	gen := s.manager.GetGeneration(generationID)
	if gen == nil {
		return fmt.Errorf("generation %s not found", generationID)
	}

	// Revert to validated state
	if err := gen.SetState(GenerationStateValidated); err != nil {
		return fmt.Errorf("failed to revert to validated: %w", err)
	}

	// If this was the active generation, unset it
	if s.manager.ActiveGeneration() != nil && s.manager.ActiveGeneration().ID == generationID {
		s.manager.mu.Lock()
		s.manager.active = nil
		s.manager.mu.Unlock()
	}

	return nil
}
