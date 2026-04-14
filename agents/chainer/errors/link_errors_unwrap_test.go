package errors

import (
	"errors"
	"testing"
)

// TestLinkDecodeErrorUnwrap tests the Unwrap method for LinkDecodeError
func TestLinkDecodeErrorUnwrap(t *testing.T) {
	t.Run("nil error unwrap", func(t *testing.T) {
		var nilErr *LinkDecodeError
		if nilErr.Unwrap() != nil {
			t.Error("expected nil from nil error unwrap")
		}
	})

	t.Run("error with cause unwrap", func(t *testing.T) {
		cause := errors.New("parse failed")
		err := &LinkDecodeError{
			LinkName:     "test-link",
			ResponseText: "bad json",
			Cause:        cause,
		}
		unwrapped := err.Unwrap()
		if unwrapped != cause {
			t.Error("expected to unwrap to cause")
		}
		if !errors.Is(err, cause) {
			t.Error("expected errors.Is to work with cause")
		}
	})

	t.Run("error without cause unwrap", func(t *testing.T) {
		err := &LinkDecodeError{
			LinkName:     "test-link",
			ResponseText: "bad json",
			Cause:        nil,
		}
		unwrapped := err.Unwrap()
		if unwrapped != nil {
			t.Error("expected nil unwrap when no cause")
		}
	})
}

// TestLinkValidationErrorUnwrap tests the Unwrap method for LinkValidationError
func TestLinkValidationErrorUnwrap(t *testing.T) {
	t.Run("nil error unwrap", func(t *testing.T) {
		var nilErr *LinkValidationError
		if nilErr.Unwrap() != nil {
			t.Error("expected nil from nil error unwrap")
		}
	})

	t.Run("error with validation error unwrap", func(t *testing.T) {
		validationErr := errors.New("schema mismatch: missing field 'name'")
		err := &LinkValidationError{
			LinkName:       "validate-link",
			OutputKey:      "result",
			ParsedOutput:   map[string]any{},
			ExpectedSchema: `{"type":"object","properties":{"name":{"type":"string"}}}`,
			ValidationErr:  validationErr,
		}
		unwrapped := err.Unwrap()
		if unwrapped != validationErr {
			t.Error("expected to unwrap to validation error")
		}
		if !errors.Is(err, validationErr) {
			t.Error("expected errors.Is to work with validation error")
		}
	})
}

// TestLinkApplyErrorUnwrap tests the Unwrap method for LinkApplyError
func TestLinkApplyErrorUnwrap(t *testing.T) {
	t.Run("nil error unwrap", func(t *testing.T) {
		var nilErr *LinkApplyError
		if nilErr.Unwrap() != nil {
			t.Error("expected nil from nil error unwrap")
		}
	})

	t.Run("error with cause unwrap", func(t *testing.T) {
		cause := errors.New("context locked for writing")
		err := &LinkApplyError{
			LinkName:    "apply-link",
			OutputKey:   "output",
			OutputValue: "test value",
			Cause:       cause,
		}
		unwrapped := err.Unwrap()
		if unwrapped != cause {
			t.Error("expected to unwrap to cause")
		}
		if !errors.Is(err, cause) {
			t.Error("expected errors.Is to work with cause")
		}
	})
}

// TestErrorTypesCompatibility ensures all error types implement error interface properly
func TestErrorTypesCompatibility(t *testing.T) {
	t.Run("LinkDecodeError is error", func(t *testing.T) {
		var _ error = &LinkDecodeError{}
	})

	t.Run("LinkValidationError is error", func(t *testing.T) {
		var _ error = &LinkValidationError{}
	})

	t.Run("LinkApplyError is error", func(t *testing.T) {
		var _ error = &LinkApplyError{}
	})
}
