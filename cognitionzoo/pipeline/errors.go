package pipeline

import "fmt"

// DecodeError reports a stage output that could not be decoded into the
// declared typed contract.
type DecodeError struct {
	Stage    string
	Contract string
	Cause    error
}

func (e *DecodeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("pipeline decode failed: stage=%s contract=%s", e.Stage, e.Contract)
	}
	return fmt.Sprintf("pipeline decode failed: stage=%s contract=%s: %v", e.Stage, e.Contract, e.Cause)
}

func (e *DecodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ValidationError reports a typed payload that did not satisfy contract rules.
type ValidationError struct {
	Stage    string
	Contract string
	Field    string
	Message  string
	Cause    error
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Field != "" {
		return fmt.Sprintf("pipeline validation failed: stage=%s contract=%s field=%s: %s", e.Stage, e.Contract, e.Field, e.Message)
	}
	if e.Message != "" {
		return fmt.Sprintf("pipeline validation failed: stage=%s contract=%s: %s", e.Stage, e.Contract, e.Message)
	}
	if e.Cause != nil {
		return fmt.Sprintf("pipeline validation failed: stage=%s contract=%s: %v", e.Stage, e.Contract, e.Cause)
	}
	return fmt.Sprintf("pipeline validation failed: stage=%s contract=%s", e.Stage, e.Contract)
}

func (e *ValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ApplyError reports a failure while projecting decoded output into shared state.
type ApplyError struct {
	Stage    string
	Contract string
	Cause    error
}

func (e *ApplyError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("pipeline apply failed: stage=%s contract=%s", e.Stage, e.Contract)
	}
	return fmt.Sprintf("pipeline apply failed: stage=%s contract=%s: %v", e.Stage, e.Contract, e.Cause)
}

func (e *ApplyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
