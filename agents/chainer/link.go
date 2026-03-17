package chainer

import (
	"errors"
	"fmt"
)

// ParseFunc transforms raw LLM output into a state value.
type ParseFunc func(responseText string) (any, error)

// FailurePolicy controls parser failure handling.
type FailurePolicy string

const (
	// FailurePolicyRetry retries a parse failure once or up to MaxRetries.
	FailurePolicyRetry FailurePolicy = "retry"
	// FailurePolicyFailFast returns immediately on parse failure.
	FailurePolicyFailFast FailurePolicy = "fail_fast"
)

// TransitionKind defines how chain execution continues after this link.
type TransitionKind string

const (
	// TransitionKindNext continues to the next link (default).
	TransitionKindNext TransitionKind = "next"
	// TransitionKindStop halts execution.
	TransitionKindStop TransitionKind = "stop"
	// TransitionKindSkipOnError skips to a specific link on error.
	TransitionKindSkipOnError TransitionKind = "skip_on_error"
)

// ErrLinkParseFailure is returned when a link response cannot be parsed.
var ErrLinkParseFailure = errors.New("chainer: link parse failure")

// Link is one isolated prompt execution step.
type Link struct {
	Name         string
	SystemPrompt string
	InputKeys    []string
	OutputKey    string
	Parse        ParseFunc
	OnFailure    FailurePolicy
	MaxRetries   int
	// Phase 5: Validation & Transitions
	Schema     string          // JSON Schema for output validation (optional)
	Transition TransitionKind  // How to continue after this link
	SkipTo     string          // Link name to skip to on error (if Transition == SkipOnError)
	// Phase 6: Tool Integration & Capabilities
	AllowedTools []string // Restrict which tools this link can access (optional; empty = all allowed)
	RequiredTools []string // Tools this link must have access to (optional)
}

// Chain is an ordered list of links.
type Chain struct {
	Links []Link
}

// NewLink constructs a generic link.
func NewLink(name, systemPrompt string, inputKeys []string, outputKey string, parse ParseFunc) Link {
	return Link{
		Name:         name,
		SystemPrompt: systemPrompt,
		InputKeys:    append([]string{}, inputKeys...),
		OutputKey:    outputKey,
		Parse:        parse,
		OnFailure:    FailurePolicyRetry,
		MaxRetries:   1,
	}
}

// NewSummarizeLink stores raw text output.
func NewSummarizeLink(name string, inputKeys []string, outputKey string) Link {
	return NewLink(name, "Summarize the available input.", inputKeys, outputKey, nil)
}

// NewTransformLink constructs a parsing transform link.
func NewTransformLink(name string, inputKeys []string, outputKey string, parse ParseFunc) Link {
	return NewLink(name, "Transform the available input.", inputKeys, outputKey, parse)
}

// Validate checks chain structural correctness.
func (c *Chain) Validate() error {
	if c == nil {
		return fmt.Errorf("chainer: chain required")
	}
	for _, link := range c.Links {
		if link.Name == "" {
			return fmt.Errorf("chainer: link name required")
		}
		if link.OutputKey == "" {
			return fmt.Errorf("chainer: link output key required")
		}
		for _, inputKey := range link.InputKeys {
			if inputKey == link.OutputKey {
				return fmt.Errorf("chainer: link %s output key cannot reference itself", link.Name)
			}
		}
	}
	return nil
}
