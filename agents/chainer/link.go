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
