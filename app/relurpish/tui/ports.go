package tui

import (
	"context"
	"errors"

	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
)

// Ports define cohesive consumer interfaces for the TUI.

// EvidenceKind identifies the type of evidence.
type EvidenceKind string

const (
	// EvidenceKindPreview indicates deterministic preview resolution.
	EvidenceKindPreview EvidenceKind = "preview"
	// EvidenceKindRuntime indicates actual runtime resolution.
	EvidenceKindRuntime EvidenceKind = "runtime"
	// EvidenceKindActualCall indicates final transport-level evidence.
	EvidenceKindActualCall EvidenceKind = "actual_call"
)
// These are narrow, typed interfaces that views depend on.
// The runtime owns the implementations; the TUI never imports runtime.

// ExecutionPort provides execution submission and observation.
type ExecutionPort interface {
	// SubmitInstruction submits an instruction for execution.
	// Returns the submission outcome with receipt and operation ID.
	SubmitInstruction(ctx context.Context, cmd InstructionCommand) (SubmissionOutcome, error)
	
	// CancelRun requests cancellation of a running execution.
	// Returns the command receipt.
	CancelRun(ctx context.Context, cmd CancelCommand) (CommandReceipt, error)
	
	// ObserveRun starts observing a run from a cursor position.
	// Returns initial snapshot and observation channel.
	ObserveRun(ctx context.Context, runID string, afterCursor uint64) (ObservationSnapshot, <-chan ObservationEvent, error)
	
	// ReadRunEvents reads events from a run starting after a cursor.
	ReadRunEvents(ctx context.Context, runID string, after uint64, limit int) ([]ObservationEvent, uint64, error)
}

// InstructionCommand represents a command to submit an instruction.
type InstructionCommand struct {
	// Text is the instruction text.
	Text string
	
	// Target is the optional target (e.g., specific run or workflow).
	Target string
	
	// Context contains additional context for the instruction.
	Context map[string]interface{}
}

// CancelCommand represents a command to cancel a run.
type CancelCommand struct {
	// RunID is the ID of the run to cancel.
	RunID string
	
	// Reason is an optional reason for cancellation.
	Reason string
}

// SubmissionOutcome represents the outcome of submitting an instruction.
type SubmissionOutcome struct {
	// Status is the receipt status (accepted, rejected, etc.).
	Status string
	
	// Receipt contains the durable receipt if accepted.
	Receipt CommandReceipt
	
	// OperationID is the operation ID if accepted.
	OperationID string
	
	// Conflict indicates if there was a digest conflict.
	Conflict bool
	
	// Error contains any error that occurred.
	Error error
}

// CommandReceipt represents a durable command receipt.
type CommandReceipt struct {
	// RequestID is the client-generated request ID.
	RequestID string
	
	// Digest is the canonical payload digest.
	Digest string
	
	// Target is the command target.
	Target string
	
	// Status is the receipt status.
	Status string
	
	// OperationID is the operation ID.
	OperationID string
	
	// AdmittedAt is when the command was admitted.
	AdmittedAt string
	
	// ProtectedUntil is when the receipt is protected until.
	ProtectedUntil string
}

// ObservationEvent represents a committed observation from the journal.
type ObservationEvent struct {
	// Cursor is the monotonic position.
	Cursor uint64
	
	// RunID identifies the run.
	RunID string
	
	// Kind identifies the observation type.
	Kind string
	
	// Payload contains the observation data.
	Payload map[string]interface{}
	
	// OccurredAt is when the observation was generated.
	OccurredAt string
	
	// CommittedAt is when the observation was durably committed.
	CommittedAt string
}

// ObservationSnapshot represents the initial state at a cursor position.
type ObservationSnapshot struct {
	// Cursor is the position.
	Cursor uint64
	
	// RunID identifies the run.
	RunID string
	
	// State contains the aggregated state at this cursor.
	State map[string]interface{}
	
	// Latest contains the most recent observations for each kind.
	Latest map[string]ObservationEvent
}

// RecipePort provides recipe operations.
type RecipePort interface {
	// OpenRecipe opens a recipe for editing.
	OpenRecipe(ctx context.Context, path string) (RecipeSnapshot, error)
	
	// EditRecipe applies edits to a recipe.
	EditRecipe(ctx context.Context, edits RecipeEditTransaction) (RecipeEditResult, error)
	
	// SaveRecipe saves a recipe.
	SaveRecipe(ctx context.Context, cmd SaveRecipeCommand) (CommandReceipt, error)
	
	// ValidateRecipe validates a recipe.
	ValidateRecipe(ctx context.Context, path string) (RecipeValidation, error)
	
	// RunRecipe triggers execution of a saved recipe.
	RunRecipe(ctx context.Context, path string) (SubmissionOutcome, error)
}

// RecipeSnapshot represents a recipe at a point in time.
type RecipeSnapshot struct {
	// Path is the recipe path.
	Path string
	
	// Content is the recipe content.
	Content string
	
	// Revision is the revision identifier.
	Revision string
	
	// Valid indicates if the recipe is syntactically valid.
	Valid bool
	
	// Errors contains validation errors if not valid.
	Errors []string
}

// RecipeEditTransaction represents a set of edits to apply.
type RecipeEditTransaction struct {
	// Path is the recipe path.
	Path string
	
	// BaseRevision is the revision to edit from.
	BaseRevision string
	
	// Edits is the list of edits to apply.
	Edits []RecipeEdit
	
	// ExpectedChecksum is the expected checksum for optimistic concurrency.
	ExpectedChecksum string
}

// RecipeEdit represents a single edit operation.
type RecipeEdit struct {
	// Kind is the edit kind (insert, delete, replace).
	Kind string
	
	// Position is the byte position.
	Position int
	
	// OldText is the text to replace (for replace operations).
	OldText string
	
	// NewText is the new text.
	NewText string
}

// RecipeEditResult represents the result of editing a recipe.
type RecipeEditResult struct {
	// Success indicates if the edit was applied.
	Success bool
	
	// NewRevision is the new revision after the edit.
	NewRevision string
	
	// NewChecksum is the new checksum.
	NewChecksum string
	
	// Errors contains any errors.
	Errors []string
}

// SaveRecipeCommand represents a command to save a recipe.
type SaveRecipeCommand struct {
	// Path is the recipe path.
	Path string
	
	// Content is the recipe content.
	Content string
	
	// Revision is the revision to save.
	Revision string
	
	// ExpectedChecksum is the expected checksum for optimistic concurrency.
	ExpectedChecksum string
}

// RecipeValidation represents the result of validating a recipe.
type RecipeValidation struct {
	// Valid indicates if the recipe is valid.
	Valid bool
	
	// Errors contains validation errors.
	Errors []string
	
	// Warnings contains validation warnings.
	Warnings []string
}

// SettingsPort provides settings operations.
type SettingsPort interface {
	// ReadSettings reads the current settings.
	ReadSettings(ctx context.Context) (SettingsSnapshot, error)
	
	// ValidateSettings validates proposed settings.
	ValidateSettings(ctx context.Context, draft SettingsDraft) (SettingsValidation, error)
	
	// SaveSettings saves settings.
	SaveSettings(ctx context.Context, cmd SettingsCommand) (CommandReceipt, error)
	
	// ActivateSettings activates staged settings.
	ActivateSettings(ctx context.Context, generation int64) (CommandReceipt, error)
}

// SettingsSnapshot represents the current settings state.
type SettingsSnapshot struct {
	// Generation is the active generation.
	Generation int64
	
	// StagedGeneration is the staged generation if any.
	StagedGeneration int64
	
	// Values contains the current settings values.
	Values map[string]interface{}
	
	// Effective contains the effective settings (merged from all sources).
	Effective map[string]interface{}
	
	// Readiness contains readiness information.
	Readiness map[string]interface{}
}

// SettingsDraft represents proposed settings changes.
type SettingsDraft struct {
	// Generation is the generation to modify.
	Generation int64
	
	// Changes contains the settings changes.
	Changes map[string]interface{}
}

// SettingsValidation represents the result of validating settings.
type SettingsValidation struct {
	// Valid indicates if the settings are valid.
	Valid bool
	
	// Errors contains validation errors.
	Errors []string
	
	// Warnings contains validation warnings.
	Warnings []string
}

// SettingsCommand represents a command to save settings.
type SettingsCommand struct {
	// Generation is the generation to save.
	Generation int64
	
	// Changes contains the settings changes.
	Changes map[string]interface{}
	
	// ExpectedChecksum is the expected checksum for optimistic concurrency.
	ExpectedChecksum string
}

// EvidencePort provides evidence inspection operations.
type EvidencePort interface {
	// GetPromptEvidence retrieves prompt evidence for a given ID and kind.
	GetPromptEvidence(ctx context.Context, promptID string, kind string) (PromptEvidenceSnapshot, error)
	
	// GetModelAttemptEvidence retrieves model attempt evidence for a given call ID.
	GetModelAttemptEvidence(ctx context.Context, callID string) (ModelAttemptSnapshot, error)
	
	// GetKnowledgeFlowEvidence retrieves knowledge flow evidence for a given operation ID.
	GetKnowledgeFlowEvidence(ctx context.Context, operationID string) (KnowledgeFlowSnapshot, error)
	
	// ListPromptEvidence lists all prompt evidence IDs.
	ListPromptEvidence(ctx context.Context) ([]string, error)
	
	// ListModelAttempts lists all model attempt IDs.
	ListModelAttempts(ctx context.Context) ([]string, error)
	
	// ListKnowledgeFlows lists all knowledge flow IDs.
	ListKnowledgeFlows(ctx context.Context) ([]string, error)
}

// PromptEvidenceSnapshot represents prompt evidence for display.
type PromptEvidenceSnapshot struct {
	// ID is the prompt identifier.
	ID string
	
	// Kind distinguishes preview, runtime, or actual call.
	Kind string
	
	// SourceReference identifies the source.
	SourceReference string
	
	// SourceRevision is the revision at resolution time.
	SourceRevision string
	
	// ResolvedAt is when resolution occurred.
	ResolvedAt string
	
	// IncludedBlocks is the count of included blocks.
	IncludedBlocks int
	
	// ExcludedBlocks is the count of excluded blocks.
	ExcludedBlocks int
	
	// OutputLength is the length of the resolved output.
	OutputLength int
	
	// DurationMS is the resolution duration.
	DurationMS int64
	
	// Error contains any resolution error.
	Error string
	
	// CallID links to the associated call.
	CallID string
	
	// AttemptID links to the associated attempt.
	AttemptID string
	
	// IsPreview indicates this is preview evidence only.
	IsPreview bool
	
	// IsActualCall indicates this is actual call evidence.
	IsActualCall bool
}

// ModelAttemptSnapshot represents model attempt evidence for display.
type ModelAttemptSnapshot struct {
	// CallID is the logical call identifier.
	CallID string
	
	// AttemptID is the physical attempt identifier.
	AttemptID string
	
	// Model identifies the model used.
	Model string
	
	// Provider identifies the provider.
	Provider string
	
	// PromptID links to the prompt evidence.
	PromptID string
	
	// InputTokens is the estimated input token count.
	InputTokens int
	
	// OutputTokens is the estimated output token count.
	OutputTokens int
	
	// FinishReason is the model's finish reason.
	FinishReason string
	
	// DurationMS is the attempt duration.
	DurationMS int64
	
	// StartedAt is when the attempt started.
	StartedAt string
	
	// CompletedAt is when the attempt completed.
	CompletedAt string
	
	// Error contains any attempt error.
	Error string
	
	// TransportReclaimed indicates if transport was reclaimed.
	TransportReclaimed bool
}

// KnowledgeFlowSnapshot represents knowledge flow evidence for display.
type KnowledgeFlowSnapshot struct {
	// ID identifies this knowledge flow.
	ID string
	
	// Kind identifies the flow type.
	Kind string
	
	// Direction is "input" or "output".
	Direction string
	
	// Operation is the specific operation.
	Operation string
	
	// ItemsSelected is the number of items.
	ItemsSelected int
	
	// BudgetUsed is the budget consumed.
	BudgetUsed int
	
	// DurationMS is the operation duration.
	DurationMS int64
	
	// StartedAt is when the operation started.
	StartedAt string
	
	// CompletedAt is when the operation completed.
	CompletedAt string
	
	// Error contains any operation error.
	Error string
}

// DoctorPort provides Doctor operations.
type DoctorPort interface {
	// GetReport gets the current Doctor report.
	GetReport(ctx context.Context) (DoctorReport, error)
	
	// Recheck requests a recheck of specific checks.
	Recheck(ctx context.Context, checkIDs []string) (DoctorReport, error)
	
	// Recover performs recovery operations.
	Recover(ctx context.Context, operationID string) (CommandReceipt, error)
}

// DoctorReport represents the Doctor report.
type DoctorReport struct {
	// Status is the overall status.
	Status string
	
	// Checks contains the individual check results.
	Checks []DoctorCheck
	
	// Warnings contains warnings.
	Warnings []string
	
	// Errors contains errors.
	Errors []string
}

// DoctorCheck represents a single Doctor check.
type DoctorCheck struct {
	// ID is the check identifier.
	ID string
	
	// Name is the human-readable name.
	Name string
	
	// Status is the check status.
	Status string
	
	// Details contains check details.
	Details string
}

// NoOpPort is a no-op implementation of ExecutionPort for use when
// the runtime is not available.
type NoOpPort struct{}

// Ensure NoOpPort implements ExecutionPort.
var _ ExecutionPort = (*NoOpPort)(nil)

func (p *NoOpPort) SubmitInstruction(ctx context.Context, cmd InstructionCommand) (SubmissionOutcome, error) {
	return SubmissionOutcome{Status: "unavailable"}, errors.New("execution not available")
}

func (p *NoOpPort) CancelRun(ctx context.Context, cmd CancelCommand) (CommandReceipt, error) {
	return CommandReceipt{}, errors.New("execution not available")
}

func (p *NoOpPort) ObserveRun(ctx context.Context, runID string, afterCursor uint64) (ObservationSnapshot, <-chan ObservationEvent, error) {
	return ObservationSnapshot{}, nil, errors.New("execution not available")
}

func (p *NoOpPort) ReadRunEvents(ctx context.Context, runID string, after uint64, limit int) ([]ObservationEvent, uint64, error) {
	return nil, 0, errors.New("execution not available")
}

// RuntimeAdapterForwarder wraps an agentlifecycle.Repository to provide
// a forward-compatible interface. This is a transitional type and will be
// removed once all callers use the new ports.
type RuntimeAdapterForwarder struct {
	repo agentlifecycle.Repository
}

// NewRuntimeAdapterForwarder creates a new forwarder.
func NewRuntimeAdapterForwarder(repo agentlifecycle.Repository) *RuntimeAdapterForwarder {
	return &RuntimeAdapterForwarder{repo: repo}
}
