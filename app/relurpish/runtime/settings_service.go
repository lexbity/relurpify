package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/securefile"
)

// SettingsService provides typed ports for configuration operations (P05).
// It implements the SettingsPort interface from tui/ports.go.
type SettingsService struct {
	workspace   string
	genManager *GenerationManager
	configPath string
}

// NewSettingsService creates a new settings service for a workspace.
func NewSettingsService(workspace string, genManager *GenerationManager, configPath string) *SettingsService {
	return &SettingsService{
		workspace:   workspace,
		genManager: genManager,
		configPath: configPath,
	}
}

// SettingsQuery represents a query for settings.
type SettingsQuery struct {
	// Section is the configuration section to query (optional).
	Section string
	// Generation is the generation to query (optional, defaults to active).
	Generation string
}

// SettingsSnapshot represents a snapshot of the current settings.
type SettingsSnapshot struct {
	// Workspace is the workspace path.
	Workspace string
	// ConfigPath is the path to the workspace configuration.
	ConfigPath string
	// Config is the parsed workspace configuration.
	Config *config.WorkspaceConfig
	// ActiveGeneration is the currently active generation ID.
	ActiveGeneration string
	// StagedGeneration is the staged generation ID (if any).
	StagedGeneration string
	// Readiness describes the overall readiness state.
	Readiness SettingsReadiness
	// Diagnostics contains any diagnostic messages.
	Diagnostics []string
}

// SettingsReadiness describes the readiness state of settings.
type SettingsReadiness struct {
	// Ready indicates if the workspace is ready.
	Ready bool
	// Degraded indicates if there are degraded but non-blocking issues.
	Degraded bool
	// BlockingIssues lists issues that block activation.
	BlockingIssues []string
	// Warnings lists non-blocking warnings.
	Warnings []string
}

// SettingsDraft represents a draft of settings changes.
type SettingsDraft struct {
	// Files contains the new file contents.
	Files map[string][]byte
	// ExpectedDigests contains the expected current digests for CAS.
	ExpectedDigests map[string]string
	// ValidateOnly indicates if this is a dry-run.
	ValidateOnly bool
}

// SettingsReview represents a reviewed settings change.
type SettingsReview struct {
	// ReviewID is a unique identifier for this review.
	ReviewID string
	// Valid indicates if the settings are valid.
	Valid bool
	// ValidationErrors contains validation errors.
	ValidationErrors []string
	// Warnings contains warnings.
	Warnings []string
	// FilesChanged lists the files that were changed.
	FilesChanged []string
	// OldHashes maps file paths to their previous hashes.
	OldHashes map[string]string
	// NewHashes maps file paths to their new hashes.
	NewHashes map[string]string
	// ReadyForActivation indicates if the changes are ready to be activated.
	ReadyForActivation bool
	// RequiresRestart indicates if activation requires a runtime restart.
	RequiresRestart bool
	// ActivationPlan describes what will happen during activation.
	ActivationPlan string
	// CreatedAt is when the review was created.
	CreatedAt time.Time
}

// CommandReceipt represents the receipt for a settings command.
type CommandReceipt struct {
	// RequestID is the unique request identifier.
	RequestID string
	// CommandType is the type of command executed.
	CommandType string
	// Success indicates if the command succeeded.
	Success bool
	// Error contains any error message.
	Error string
	// GenerationID is the generation affected by the command.
	GenerationID string
	// Timestamp is when the command was executed.
	Timestamp time.Time
	// Details contains additional command-specific details.
	Details map[string]any
}

// ReadSettings returns the current settings snapshot.
func (s *SettingsService) ReadSettings(ctx context.Context, query SettingsQuery) (SettingsSnapshot, error) {
	snapshot := SettingsSnapshot{
		Workspace:   s.workspace,
		ConfigPath: s.configPath,
	}

	// Load the current configuration
	wsConfig, err := config.LoadWorkspaceConfig("", s.workspace, config.WorkspaceLoadOptions{})
	if err != nil {
		// Config may not exist yet
		snapshot.Diagnostics = append(snapshot.Diagnostics, 
			fmt.Sprintf("workspace config not found: %v", err))
	} else {
		snapshot.Config = wsConfig
	}

	// Get generation information
	snapshot.ActiveGeneration = ""
	if active := s.genManager.ActiveGeneration(); active != nil {
		snapshot.ActiveGeneration = active.ID
	}

	// Check for staged generation (simplified for P05)
	// In a full implementation, this would check for staged but not activated generations
	snapshot.StagedGeneration = ""

	// Check readiness
	snapshot.Readiness = s.checkReadiness(ctx, wsConfig)

	return snapshot, nil
}

// ValidateSettings validates a settings draft.
func (s *SettingsService) ValidateSettings(ctx context.Context, draft SettingsDraft) (SettingsValidation, error) {
	validation := SettingsValidation{
		Valid:   true,
		Errors: make([]string, 0),
		Warnings: make([]string, 0),
	}

	// For P05, we validate that:
	// 1. All paths are within the workspace
	// 2. No forbidden patterns are present
	// 3. Schema validation passes

	for path, content := range draft.Files {
		// Check path is within workspace
		if !isConfinedPath(s.workspace, path) {
			validation.Valid = false
			validation.Errors = append(validation.Errors,
				fmt.Sprintf("path %s escapes workspace confinement", path))
		}

		// Check for forbidden secret fields (basic check)
		contentStr := string(content)
		if strings.Contains(contentStr, "password:") ||
			strings.Contains(contentStr, "secret:") ||
			strings.Contains(contentStr, "api_key:") {
			validation.Warnings = append(validation.Warnings,
				fmt.Sprintf("potential secret detected in %s", path))
		}
	}

	// Try to parse and validate the configuration
	if len(draft.Files) > 0 {
		// For workspace.yaml - simplified validation for P05
		if wsContent, ok := draft.Files[config.DefaultWorkspaceConfigPath(s.workspace)]; ok {
			// Try to parse as workspace config
			var wsConfig config.WorkspaceConfig
			if err := yaml.Unmarshal(wsContent, &wsConfig); err != nil {
				validation.Valid = false
				validation.Errors = append(validation.Errors,
					fmt.Sprintf("workspace config validation failed: %v", err))
			}
		}
	}

	return validation, nil
}

// PrepareSettingsSave prepares a settings save operation.
func (s *SettingsService) PrepareSettingsSave(ctx context.Context, draft SettingsDraft) (SettingsReview, error) {
	review := SettingsReview{
		ReviewID:     fmt.Sprintf("review_%d", time.Now().UnixNano()),
		Valid:        true,
		CreatedAt:    time.Now().UTC(),
		FilesChanged: make([]string, 0),
		OldHashes:     make(map[string]string),
		NewHashes:     make(map[string]string),
	}

	// Validate first
	validation, err := s.ValidateSettings(ctx, draft)
	if err != nil {
		return review, err
	}

	if !validation.Valid {
		review.Valid = false
		review.ValidationErrors = validation.Errors
		review.ReadyForActivation = false
		return review, nil
	}

	// Check current hashes for CAS
	for path := range draft.Files {
		currentContent, err := osReadFileFunc(path)
		if err != nil && !osIsNotExistFunc(err) {
			review.Valid = false
			review.ValidationErrors = append(review.ValidationErrors,
				fmt.Sprintf("failed to read current file %s: %v", path, err))
			continue
		}

		var currentHash string
		if err == nil {
			currentHash = securefile.ComputeDigest(currentContent)
		}

		// Check expected digest
		if expected, ok := draft.ExpectedDigests[path]; ok {
			if expected != currentHash {
				review.Valid = false
				review.ValidationErrors = append(review.ValidationErrors,
					fmt.Sprintf("CAS conflict for %s: expected %s, got %s",
						path, expected, currentHash))
			}
		}

		review.OldHashes[path] = currentHash
		newContent := draft.Files[path]
		newHash := securefile.ComputeDigest(newContent)
		review.NewHashes[path] = newHash
		review.FilesChanged = append(review.FilesChanged, path)
	}

	if review.Valid {
		review.ReadyForActivation = true
		review.ActivationPlan = s.buildActivationPlan(draft)
	}

	return review, nil
}

// CommitSettings commits the settings changes.
func (s *SettingsService) CommitSettings(ctx context.Context, cmd SettingsCommand) (CommandReceipt, error) {
	receipt := CommandReceipt{
		RequestID:   fmt.Sprintf("req_%d", time.Now().UnixNano()),
		CommandType: "settings_commit",
		Timestamp:   time.Now().UTC(),
		Details:     make(map[string]any),
	}

	// Create a multi-file transaction
	txn := securefile.NewMultiFileTransaction()

	for path, content := range cmd.Files {
		txn.AddFile(path, content)
		if expected, ok := cmd.ExpectedDigests[path]; ok {
			txn.WithExpectedDigest(path, expected)
		}
		// For P05, backups are created with timestamp
		backupPath := filepath.Join(filepath.Dir(path), "backups", 
			filepath.Base(path)+"."+time.Now().UTC().Format("20060102_150405")+".bak")
		txn.WithBackup(path, backupPath)
	}

	// Execute the transaction
	if err := txn.Execute(); err != nil {
		receipt.Success = false
		receipt.Error = err.Error()
		return receipt, nil
	}

	// Create a new generation snapshot
	wsConfig, err := config.LoadWorkspaceConfig("", s.workspace, config.WorkspaceLoadOptions{})
	if err != nil {
		// If config doesn't exist yet, that's okay
		wsConfig = nil
	}

	gen := s.genManager.CreateGenerationFromConfig(wsConfig)
	receipt.GenerationID = gen.ID

	// Mark as staged (not yet activated)
	if err := gen.SetState(GenerationStateValidated); err != nil {
		receipt.Success = false
		receipt.Error = err.Error()
		return receipt, nil
	}

	receipt.Success = true
	receipt.Details["files_changed"] = len(cmd.Files)
	receipt.Details["generation_id"] = gen.ID

	return receipt, nil
}

// ActivateSettings activates the staged settings.
func (s *SettingsService) ActivateSettings(ctx context.Context, cmd ActivationCommand) (CommandReceipt, error) {
	receipt := CommandReceipt{
		RequestID:   fmt.Sprintf("req_%d", time.Now().UnixNano()),
		CommandType: "settings_activate",
		Timestamp:   time.Now().UTC(),
		Details:     make(map[string]any),
	}

	// Get the staged generation
	gen := s.genManager.GetGeneration(cmd.StagedGeneration)
	if gen == nil {
		receipt.Success = false
		receipt.Error = "staged generation not found"
		return receipt, nil
	}

	// For P05: Check if there are active runs in the current generation
	activeGen := s.genManager.ActiveGeneration()
	if activeGen != nil {
		if activeGen.ActiveRunCount() > 0 {
			if !cmd.Force {
				// Wait for quiescence
				deadline := cmd.Deadline
				if deadline == 0 {
					deadline = DefaultQuiescenceDeadline
				}
				if !s.genManager.WaitQuiescence(activeGen.ID, deadline) {
					receipt.Success = false
					receipt.Error = "quiescence deadline reached: active runs still exist"
					return receipt, nil
				}
			} else {
				// Force activation - cancel active runs
				// In a full implementation, this would actually cancel runs
				// For P05, we just log this
				receipt.Details["force_activation"] = true
			}
		}
	}

	// Activate the new generation
	if err := gen.SetState(GenerationStateActive); err != nil {
		receipt.Success = false
		receipt.Error = err.Error()
		return receipt, nil
	}

	// Set as active in the manager
	if err := s.genManager.SetActive(gen); err != nil {
		receipt.Success = false
		receipt.Error = err.Error()
		return receipt, nil
	}

	receipt.Success = true
	receipt.GenerationID = gen.ID
	receipt.Details["previous_generation"] = ""
	if activeGen != nil {
		receipt.Details["previous_generation"] = activeGen.ID
		// Retire the old generation
		_ = s.genManager.RetireActive()
	}

	return receipt, nil
}

// checkReadiness checks the overall readiness of the workspace.
func (s *SettingsService) checkReadiness(ctx context.Context, wsConfig *config.WorkspaceConfig) SettingsReadiness {
	readiness := SettingsReadiness{
		Ready:     true,
		Degraded:  false,
		Warnings:  make([]string, 0),
		BlockingIssues: make([]string, 0),
	}

	// Check if workspace config exists
	configPath := config.DefaultWorkspaceConfigPath(s.workspace)
	if _, err := osStatFunc(configPath); osIsNotExistFunc(err) {
		readiness.Ready = false
		readiness.BlockingIssues = append(readiness.BlockingIssues,
			"workspace configuration missing")
		return readiness
	}

	// Check if workspace config is valid
	if wsConfig == nil {
		readiness.Ready = false
		readiness.BlockingIssues = append(readiness.BlockingIssues,
			"workspace configuration invalid")
		return readiness
	}

	// Check if active generation exists and is ready
	activeGen := s.genManager.ActiveGeneration()
	if activeGen == nil {
		readiness.Ready = false
		readiness.BlockingIssues = append(readiness.BlockingIssues,
			"no active generation")
		return readiness
	}

	if activeGen.state != GenerationStateActive {
		readiness.Ready = false
		readiness.BlockingIssues = append(readiness.BlockingIssues,
			fmt.Sprintf("active generation in state %s", activeGen.state))
		return readiness
	}

	// For P05, we consider the workspace ready if we have a valid config
	// and an active generation
	return readiness
}

// buildActivationPlan builds a human-readable activation plan.
func (s *SettingsService) buildActivationPlan(draft SettingsDraft) string {
	var sb strings.Builder
	sb.WriteString("Activation plan:\n")

	for path := range draft.Files {
		sb.WriteString(fmt.Sprintf("  - Update %s\n", path))
	}

	if len(draft.Files) > 0 {
		sb.WriteString("Quiescence: Wait up to ")
		sb.WriteString(DefaultQuiescenceDeadline.String())
		sb.WriteString(" for active runs to complete\n")
	}

	return sb.String()
}

// isConfinedPath checks if a path is confined within the workspace.
func isConfinedPath(workspace, path string) bool {
	// Normalize paths
	ws, err := filepath.Abs(workspace)
	if err != nil {
		return false
	}

	p, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// Ensure workspace path is clean
	ws = filepath.Clean(ws)
	p = filepath.Clean(p)

	// Check if path starts with workspace
	return strings.HasPrefix(p, ws+"/") || p == ws
}

// OS functions for testing
var (
	osReadFileFunc = os.ReadFile
	osStatFunc     = os.Stat
	osIsNotExistFunc = os.IsNotExist
)
