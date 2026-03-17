package telemetry

import (
	"time"
)

// ChainerEventKind categorizes the type of execution event.
type ChainerEventKind string

const (
	KindLinkStart       ChainerEventKind = "link_start"
	KindLinkFinish      ChainerEventKind = "link_finish"
	KindLinkError       ChainerEventKind = "link_error"
	KindParsingFailure  ChainerEventKind = "parsing_failure"
	KindRetryAttempt    ChainerEventKind = "retry_attempt"
	KindCompressionEvent ChainerEventKind = "compression_event"
	KindResumeEvent     ChainerEventKind = "resume_event"
)

// ChainerEvent represents a significant moment during ChainerAgent execution.
//
// Events are emitted at:
//   - Link start/finish/error
//   - Parse failures (recovery or retry)
//   - Retry attempts after failures
//   - Compression triggers (budget warning/exceeded)
//   - Execution resumption from checkpoint
type ChainerEvent struct {
	// Timing
	Timestamp time.Time

	// Classification
	Kind ChainerEventKind

	// Context
	TaskID    string // Task being executed
	LinkName  string // Name of link (stage) generating event
	ChainStep int    // Index in chain (0-based)

	// Execution metadata
	InputKeys   []string `json:"input_keys,omitempty"`   // Input keys declared for stage
	OutputKey   string   `json:"output_key,omitempty"`   // Output key produced by stage
	ResponseText string  `json:"response_text,omitempty"` // LLM response (truncated if long)

	// Error context (for failures)
	ErrorMessage string `json:"error_message,omitempty"`
	ErrorType    string `json:"error_type,omitempty"`

	// Retry context
	AttemptNumber int       `json:"attempt_number,omitempty"` // 1-based attempt count
	MaxRetries    int       `json:"max_retries,omitempty"`    // Max allowed retries
	RetryReason   string    `json:"retry_reason,omitempty"`   // Why retry triggered

	// Compression context
	BudgetRemaining int    `json:"budget_remaining,omitempty"` // Tokens remaining
	BudgetLimit     int    `json:"budget_limit,omitempty"`     // Total token limit
	CompressionMode string `json:"compression_mode,omitempty"` // "adaptive", "aggressive", etc.

	// Resume context
	ResumedFromStepIndex int `json:"resumed_from_step_index,omitempty"` // Step resumed from
}

// LinkStartEvent creates a LinkStart event for stage beginning.
func LinkStartEvent(taskID, linkName string, stepIndex int, inputKeys []string, outputKey string) *ChainerEvent {
	return &ChainerEvent{
		Timestamp:  time.Now(),
		Kind:       KindLinkStart,
		TaskID:     taskID,
		LinkName:   linkName,
		ChainStep:  stepIndex,
		InputKeys:  inputKeys,
		OutputKey:  outputKey,
	}
}

// LinkFinishEvent creates a LinkFinish event for successful stage completion.
func LinkFinishEvent(taskID, linkName string, stepIndex int, outputKey, response string) *ChainerEvent {
	return &ChainerEvent{
		Timestamp:    time.Now(),
		Kind:         KindLinkFinish,
		TaskID:       taskID,
		LinkName:     linkName,
		ChainStep:    stepIndex,
		OutputKey:    outputKey,
		ResponseText: truncateResponse(response, 500),
	}
}

// LinkErrorEvent creates a LinkError event for stage failure.
func LinkErrorEvent(taskID, linkName string, stepIndex int, errMsg, errType string) *ChainerEvent {
	return &ChainerEvent{
		Timestamp:    time.Now(),
		Kind:         KindLinkError,
		TaskID:       taskID,
		LinkName:     linkName,
		ChainStep:    stepIndex,
		ErrorMessage: errMsg,
		ErrorType:    errType,
	}
}

// ParsingFailureEvent creates a ParsingFailure event when LLM output parse fails.
func ParsingFailureEvent(taskID, linkName string, stepIndex int, response, errMsg string) *ChainerEvent {
	return &ChainerEvent{
		Timestamp:    time.Now(),
		Kind:         KindParsingFailure,
		TaskID:       taskID,
		LinkName:     linkName,
		ChainStep:    stepIndex,
		ResponseText: truncateResponse(response, 500),
		ErrorMessage: errMsg,
	}
}

// RetryAttemptEvent creates a RetryAttempt event when retry triggered.
func RetryAttemptEvent(taskID, linkName string, stepIndex int, attempt, maxRetries int, reason string) *ChainerEvent {
	return &ChainerEvent{
		Timestamp:     time.Now(),
		Kind:          KindRetryAttempt,
		TaskID:        taskID,
		LinkName:      linkName,
		ChainStep:     stepIndex,
		AttemptNumber: attempt,
		MaxRetries:    maxRetries,
		RetryReason:   reason,
	}
}

// CompressionEventCreate creates a CompressionEvent when budget compression triggered.
func CompressionEvent(taskID string, budgetRemaining, budgetLimit int, mode string) *ChainerEvent {
	return &ChainerEvent{
		Timestamp:         time.Now(),
		Kind:              KindCompressionEvent,
		TaskID:            taskID,
		BudgetRemaining:   budgetRemaining,
		BudgetLimit:       budgetLimit,
		CompressionMode:   mode,
	}
}

// ResumeEventCreate creates a ResumeEvent when execution resumed from checkpoint.
func ResumeEvent(taskID string, resumedFromStepIndex int) *ChainerEvent {
	return &ChainerEvent{
		Timestamp:            time.Now(),
		Kind:                 KindResumeEvent,
		TaskID:               taskID,
		ResumedFromStepIndex: resumedFromStepIndex,
	}
}

// Helper: truncate long responses to max length
func truncateResponse(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}
