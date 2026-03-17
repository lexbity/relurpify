package errors

import (
	"fmt"
)

// LinkDecodeError is returned when a link's LLM response cannot be parsed.
type LinkDecodeError struct {
	LinkName       string
	ResponseText   string
	Cause          error
	RetryCount     int
	MaxRetries     int
	LastAttempt    string
}

func (e *LinkDecodeError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf(
		"chainer: link %q decode failed after %d retries: %v (last response: %q)",
		e.LinkName, e.RetryCount, e.Cause, e.LastAttempt,
	)
}

func (e *LinkDecodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// LinkValidationError is returned when a link's parsed output doesn't match its schema.
type LinkValidationError struct {
	LinkName        string
	OutputKey       string
	ParsedOutput    any
	ExpectedSchema  string
	ValidationErr   error
	RetryCount      int
	MaxRetries      int
}

func (e *LinkValidationError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf(
		"chainer: link %q validation failed after %d retries (output key: %q, schema: %s): %v",
		e.LinkName, e.RetryCount, e.OutputKey, e.ExpectedSchema, e.ValidationErr,
	)
}

func (e *LinkValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.ValidationErr
}

// LinkApplyError is returned when a link's result cannot be written to shared context.
type LinkApplyError struct {
	LinkName    string
	OutputKey   string
	OutputValue any
	Cause       error
}

func (e *LinkApplyError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf(
		"chainer: link %q apply failed (output key: %q): %v",
		e.LinkName, e.OutputKey, e.Cause,
	)
}

func (e *LinkApplyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
