package rewoo

import "codeburg.org/lexbit/relurpify/framework/contextstream"

// StepOnFailure defines how executor failures are handled.
type StepOnFailure string

const (
	// StepOnFailureSkip records the failure and continues.
	StepOnFailureSkip StepOnFailure = "skip"
	// StepOnFailureAbort aborts execution immediately.
	StepOnFailureAbort StepOnFailure = "abort"
	// StepOnFailureReplan aborts the current run and asks the agent to replan.
	StepOnFailureReplan StepOnFailure = "replan"
)

// RewooStep is a single tool-backed plan step.
type RewooStep struct {
	ID          string         `json:"id"`
	Description string         `json:"description"`
	Tool        string         `json:"tool"`
	Params      map[string]any `json:"params"`
	DependsOn   []string       `json:"depends_on"`
	OnFailure   StepOnFailure  `json:"on_failure"`
}

// RewooPlan is the planner output consumed by the executor.
type RewooPlan struct {
	Goal  string      `json:"goal"`
	Steps []RewooStep `json:"steps"`
}

// RewooStepResult stores the mechanical outcome of one tool step.
type RewooStepResult struct {
	StepID  string         `json:"step_id"`
	Tool    string         `json:"tool"`
	Success bool           `json:"success"`
	Output  map[string]any `json:"output,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// RewooContextConfig controls context management and token budgeting.
type RewooContextConfig struct {
	// StrategyName: "adaptive" (default), "conservative", "aggressive"
	StrategyName string
	// PreferredDetailLevel: controls AST/file detail in context ("brief", "normal", "detailed")
	PreferredDetailLevel string
	// MinHistorySize: minimum interactions to retain before pruning
	MinHistorySize int
	// CompressionThreshold: (0-1) when to trigger compression (e.g., 0.8 = 80% budget used)
	CompressionThreshold float64
	// BudgetSystemTokens: reserved for system/framework overhead
	BudgetSystemTokens int
	// BudgetToolTokens: reserved for tool specs in planning context
	BudgetToolTokens int
	// BudgetOutputTokens: reserved for response generation
	BudgetOutputTokens int
}

// RewooPermissionConfig controls authorization and HITL.
type RewooPermissionConfig struct {
	// DefaultPolicy: "allow", "deny", "ask" (default)
	DefaultPolicy string
	// WorkspacePath: base path for file system permission scope
	WorkspacePath string
	// RequireApprovalForTools: if non-empty, only these tool names require explicit approval
	RequireApprovalForTools []string
	// EnableHITL: if true, unknown operations route to human-in-the-loop
	EnableHITL bool
}

// RewooGraphConfig controls graph execution and parallelism.
type RewooGraphConfig struct {
	// MaxParallelSteps: limit concurrent step execution (default 4)
	MaxParallelSteps int
	// MaxNodeVisits: cycle guard for graph execution (default 1024)
	MaxNodeVisits int
	// CheckpointInterval: checkpoint every N nodes (0 = disabled)
	CheckpointInterval int
	// EnableParallelExecution: if false, all steps run sequentially
	EnableParallelExecution bool
}

// RewooOptions controls execution limits and fallback behavior.
type RewooOptions struct {
	MaxReplanAttempts int
	OnFailure         StepOnFailure
	MaxSteps          int
	// ContextConfig controls context budgeting and management
	ContextConfig RewooContextConfig
	// PermConfig controls permissions and authorization
	PermConfig RewooPermissionConfig
	// GraphConfig controls graph-based execution
	GraphConfig RewooGraphConfig
	// Streaming trigger configuration
	StreamTrigger   *contextstream.Trigger
	StreamMode      contextstream.Mode
	StreamQuery     string
	StreamMaxTokens int
}
