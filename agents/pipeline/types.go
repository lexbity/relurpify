package pipeline

import (
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// TransitionKind indicates what the runtime should do after a stage finishes.
type TransitionKind string

const (
	TransitionNext   TransitionKind = "next"
	TransitionBranch TransitionKind = "branch"
	TransitionRetry  TransitionKind = "retry"
	TransitionStop   TransitionKind = "stop"
)

// RetryPolicy captures stage-level retry intent for later runtime phases.
type RetryPolicy struct {
	MaxAttempts            int
	RetryOnDecodeError     bool
	RetryOnValidationError bool
}

// ContractMetadata describes how a typed stage exchanges data with shared state.
type ContractMetadata struct {
	InputKey      string
	OutputKey     string
	SchemaVersion string
	AllowTools    bool
	RetryPolicy   RetryPolicy
}

// StageTransition captures the declared outcome of a completed stage.
type StageTransition struct {
	Kind      TransitionKind
	NextStage string
	Reason    string
}

// StageResult is the reusable envelope persisted or emitted by pipeline stages.
type StageResult struct {
	StageName       string
	ContractName    string
	ContractVersion string
	Prompt          string
	Response        *contracts.LLMResponse
	DecodedOutput   any
	DecodedJSON     string
	ValidationOK    bool
	ErrorText       string
	RetryAttempt    int
	StartedAt       time.Time
	FinishedAt      time.Time
	Transition      StageTransition
}
