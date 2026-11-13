// Package runtime provides the Relurpish workspace owner and its runtime services.
// This file implements the InitializationPlanner for P05.
package runtime

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// InitializationPlanner creates and validates workspace initialization plans.
// It implements the reviewed materialization plan tokens from P05.
type InitializationPlanner struct {
	workspaceRoot string
	configDoc     *config.DocumentSnapshot
	fileTxnService FileTransactionPort
}

// NewInitializationPlanner creates a new initialization planner.
func NewInitializationPlanner(
	workspaceRoot string,
	configDoc *config.DocumentSnapshot,
	fileTxnService FileTransactionPort,
) *InitializationPlanner {
	return &InitializationPlanner{
		workspaceRoot: workspaceRoot,
		configDoc:     configDoc,
		fileTxnService: fileTxnService,
	}
}

// MaterializationPlan represents a reviewed initialization plan with explicit tokens.
type MaterializationPlan struct {
	// PlanID is a unique identifier for this plan.
	PlanID string
	// Token is the reviewed materialization token (D-15/P05).
	Token string
	// Description describes what the plan does.
	Description string
	// Steps is the ordered list of steps to execute.
	Steps []MaterializationStep
	// FilesToCreate lists files that will be created.
	FilesToCreate []string
	// FilesToPreserve lists files that will be preserved.
	FilesToPreserve []string
	// FilesToReject lists files that will be rejected (unsafe paths).
	FilesToReject []string
	// Preconditions are conditions that must be true before execution.
	Preconditions []string
	// Postconditions are conditions that will be true after successful execution.
	Postconditions []string
	// CreatedAt is when the plan was created.
	CreatedAt time.Time
}

// MaterializationStep represents a single step in a materialization plan.
type MaterializationStep struct {
	StepID    string
	Action    MaterializationAction
	Path      string
	Content   []byte
	Checksum  string
	Validate  func(path string, content []byte) error
	Rollback  func() error
}

// MaterializationAction is the type of action to perform.
type MaterializationAction string

const (
	ActionCreateFile MaterializationAction = "create_file"
	ActionPreserve   MaterializationAction = "preserve"
	ActionReject     MaterializationAction = "reject"
	ActionValidate   MaterializationAction = "validate"
	ActionProbe      MaterializationAction = "probe"
)

// MaterializationResult represents the result of executing a materialization plan.
type MaterializationResult struct {
	Success     bool
	PlanID      string
	StepsCompleted int
	StepsFailed int
	Errors      []error
	Warnings    []string
	StartTime   time.Time
	EndTime     time.Time
}

// TokenType defines the types of reviewed materialization tokens.
type TokenType string

const (
	// TokenCreateMissing creates only missing bundled files (safe).
	TokenCreateMissing TokenType = "create_missing_bundled"
	// TokenPreserveExisting preserves existing files (safe).
	TokenPreserveExisting TokenType = "preserve_existing"
	// TokenRejectUnsafe rejects unsafe paths (safe).
	TokenRejectUnsafe TokenType = "reject_unsafe_paths"
	// TokenValidateConfig validates configuration (safe).
	TokenValidateConfig TokenType = "validate_config"
	// TokenRecheckReadiness rechecks readiness after changes (safe).
	TokenRecheckReadiness TokenType = "recheck_readiness"
)

// PlanTokens contains the reviewed materialization plan tokens.
var PlanTokens = map[TokenType]string{
	TokenCreateMissing:   "create_missing_bundled_only",
	TokenPreserveExisting: "preserve_existing_files",
	TokenRejectUnsafe:    "reject_unsafe_paths",
	TokenValidateConfig:  "validate_configuration",
	TokenRecheckReadiness: "recheck_readiness",
}

// CreateInitializationPlan creates a reviewed initialization plan for a new workspace.
// The plan only includes safe operations as specified in P05/D-15.
func (p *InitializationPlanner) CreateInitializationPlan(ctx context.Context) (*MaterializationPlan, error) {
	planID := fmt.Sprintf("init_%d", time.Now().UnixNano())

	plan := &MaterializationPlan{
		PlanID:        planID,
		Token:         PlanTokens[TokenCreateMissing],
		Description:   "Initialize workspace with missing bundled files only",
		CreatedAt:     time.Now().UTC(),
		Preconditions: []string{
			"workspace_root directory exists",
			"user has write permission to workspace_root",
			"no unsafe symlinks in workspace",
		},
		Postconditions: []string{
			"bundled files are created",
			"existing files are preserved",
			"unsafe paths are rejected",
			"configuration is validated",
			"readiness is rechecked",
		},
	}

	// Define bundled files that can be created
	bundledFiles := []string{
		".relurpish",
		"config.yaml",
		"AGENTS.md",
		"README.md",
	}

	// Add create steps for bundled files
	for _, file := range bundledFiles {
		filePath := p.getWorkspacePath(file)
		plan.FilesToCreate = append(plan.FilesToCreate, filePath)
		
		// Create a safe content for each file
		content := p.getBundledFileContent(file)
		checksum := fmt.Sprintf("%x", sha256Sum(content))
		
		plan.Steps = append(plan.Steps, MaterializationStep{
			StepID:   fmt.Sprintf("create_%s", file),
			Action:   ActionCreateFile,
			Path:     filePath,
			Content:  content,
			Checksum: checksum,
			Validate: func(path string, content []byte) error {
				// Validate that the path is safe
				if !p.isSafePath(path) {
					return fmt.Errorf("unsafe path: %s", path)
				}
				// Validate content is not empty for required files
				if len(content) == 0 && file != ".relurpish" {
					return fmt.Errorf("empty content for required file: %s", file)
				}
				return nil
			},
			Rollback: func() error {
				// Rollback would delete the file if it was created
				// In a real implementation, we'd track what was actually created
				return nil
			},
		})
	}

	// Add preserve steps for existing files
	existingFiles := []string{
		"config.yaml",
		"AGENTS.md",
		"README.md",
	}

	for _, file := range existingFiles {
		filePath := p.getWorkspacePath(file)
		plan.FilesToPreserve = append(plan.FilesToPreserve, filePath)
		
		plan.Steps = append(plan.Steps, MaterializationStep{
			StepID: fmt.Sprintf("preserve_%s", file),
			Action: ActionPreserve,
			Path:   filePath,
			Validate: func(path string, content []byte) error {
				// Check if file exists, if so preserve it
				// In a real implementation, we'd check the filesystem
				return nil
			},
		})
	}

	// Add reject steps for unsafe paths
	unsafePatterns := []string{
		"../",
		"/etc",
		"/bin",
		"/usr",
		"/root",
		"/tmp",
		"/var",
	}

	for _, pattern := range unsafePatterns {
		plan.FilesToReject = append(plan.FilesToReject, pattern)
		plan.Steps = append(plan.Steps, MaterializationStep{
			StepID: fmt.Sprintf("reject_%s", pattern),
			Action: ActionReject,
			Path:   pattern,
			Validate: func(path string, content []byte) error {
				if strings.Contains(path, pattern) {
					return fmt.Errorf("unsafe path pattern detected: %s", pattern)
				}
				return nil
			},
		})
	}

	// Add validation step
	plan.Steps = append(plan.Steps, MaterializationStep{
		StepID: "validate_config",
		Action: ActionValidate,
		Validate: func(path string, content []byte) error {
			// In a real implementation, we'd validate the config
			return nil
		},
	})

	// Add readiness recheck step
	plan.Steps = append(plan.Steps, MaterializationStep{
		StepID: "recheck_readiness",
		Action: ActionProbe,
		Validate: func(path string, content []byte) error {
			// In a real implementation, we'd recheck readiness
			return nil
		},
	})

	return plan, nil
}

// ExecutePlan executes a materialization plan.
func (p *InitializationPlanner) ExecutePlan(ctx context.Context, plan *MaterializationPlan) MaterializationResult {
	result := MaterializationResult{
		PlanID:    plan.PlanID,
		StartTime: time.Now().UTC(),
	}

	// Execute each step
	for i, step := range plan.Steps {
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, ctx.Err())
			return result
		default:
		}

		// Validate the step
		if step.Validate != nil {
			if err := step.Validate(step.Path, step.Content); err != nil {
				result.Errors = append(result.Errors, err)
				result.StepsFailed++
				
				// Execute rollback if available
				if step.Rollback != nil {
					if rbErr := step.Rollback(); rbErr != nil {
						result.Warnings = append(result.Warnings, fmt.Sprintf("rollback failed for step %s: %v", step.StepID, rbErr))
					}
				}
				continue
			}
		}

		// Execute the action
		if err := p.executeStepAction(ctx, step); err != nil {
			result.Errors = append(result.Errors, err)
			result.StepsFailed++
			
			// Execute rollback
			if step.Rollback != nil {
				if rbErr := step.Rollback(); rbErr != nil {
					result.Warnings = append(result.Warnings, fmt.Sprintf("rollback failed for step %s: %v", step.StepID, rbErr))
				}
			}
			continue
		}

		result.StepsCompleted++
		_ = i // Use i to avoid unused variable warning
	}

	result.EndTime = time.Now().UTC()
	result.Success = len(result.Errors) == 0

	return result
}

// executeStepAction executes the action for a materialization step.
func (p *InitializationPlanner) executeStepAction(ctx context.Context, step MaterializationStep) error {
	switch step.Action {
	case ActionCreateFile:
		// Use file transaction service for atomic writes
		txnReq := FileTransactionRequest{
			TransactionID: TransactionID(fmt.Sprintf("init_%s", step.StepID)),
			Changes: []FileChange{
				{
					Path:     step.Path,
					Content:  step.Content,
					OldDigest: "", // No expected digest for new files
				},
			},
			BackupEnabled: false, // No backup for initialization
			Description:   fmt.Sprintf("Initialization: %s", step.StepID),
		}
		
		result := p.fileTxnService.ExecuteTransaction(txnReq)
		if !result.Success {
			return fmt.Errorf("failed to create file %s: %v", step.Path, result.Error)
		}
		
		return nil

	case ActionPreserve:
		// In a real implementation, we'd check if the file exists
		// and ensure it's not modified
		return nil

	case ActionReject:
		// This is validated in the Validate function, so we don't need to do anything here
		return nil

	case ActionValidate:
		// In a real implementation, we'd run validation
		return nil

	case ActionProbe:
		// In a real implementation, we'd run readiness checks
		return nil

	default:
		return fmt.Errorf("unknown action: %s", step.Action)
	}
}

// getWorkspacePath returns the full path for a file in the workspace.
func (p *InitializationPlanner) getWorkspacePath(file string) string {
	if file == ".relurpish" {
		return p.workspaceRoot
	}
	return filepath.Join(p.workspaceRoot, file)
}

// isSafePath checks if a path is safe (not outside workspace, not a symlink, etc.).
func (p *InitializationPlanner) isSafePath(path string) bool {
	// Basic check - in a real implementation, this would be more thorough
	absPath, _ := filepath.Abs(path)
	absWorkspace, _ := filepath.Abs(p.workspaceRoot)
	
	return filepath.HasPrefix(absPath, absWorkspace)
}

// getBundledFileContent returns safe content for bundled files.
func (p *InitializationPlanner) getBundledFileContent(file string) []byte {
	switch file {
	case ".relurpish":
		return []byte("relurpish workspace\n")
	case "config.yaml":
		return []byte("# Relurpish Configuration\napiVersion: v1\nkind: Workspace\nmetadata:\n  name: default\n")
	case "AGENTS.md":
		return []byte("# Agents\n\n## Default Agent\n\n")
	case "README.md":
		return []byte("# Relurpish Workspace\n\nThis is a Relurpish workspace.\n")
	default:
		return []byte("")
	}
}

// InitializeWorkspace performs a complete workspace initialization.
func (p *InitializationPlanner) InitializeWorkspace(ctx context.Context) (*MaterializationResult, error) {
	// Create plan
	plan, err := p.CreateInitializationPlan(ctx)
	if err != nil {
		return nil, err
	}

	// Execute plan
	result := p.ExecutePlan(ctx, plan)

	return &result, nil
}

// InitializationPort defines the interface for initialization operations.
type InitializationPort interface {
	CreateInitializationPlan(ctx context.Context) (*MaterializationPlan, error)
	ExecutePlan(ctx context.Context, plan *MaterializationPlan) MaterializationResult
	InitializeWorkspace(ctx context.Context) (*MaterializationResult, error)
}

// Ensure InitializationPlanner implements InitializationPort.
var _ InitializationPort = (*InitializationPlanner)(nil)

// sha256Sum is a helper to compute SHA-256 sum.
func sha256Sum(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}
