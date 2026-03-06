package browser

import (
	"errors"
	"fmt"
)

// ErrorCode is the normalized Relurpify browser error code.
type ErrorCode string

const (
	ErrUnknownOperation       ErrorCode = "unknown_operation"
	ErrUnsupportedOperation   ErrorCode = "unsupported_operation"
	ErrNoSuchElement          ErrorCode = "no_such_element"
	ErrStaleElement           ErrorCode = "stale_element_reference"
	ErrElementNotInteractable ErrorCode = "element_not_interactable"
	ErrTimeout                ErrorCode = "timeout"
	ErrNavigationBlocked      ErrorCode = "navigation_blocked"
	ErrScriptEvaluation       ErrorCode = "script_evaluation_failed"
	ErrBackendDisconnected    ErrorCode = "backend_disconnected"
	ErrInvalidURL             ErrorCode = "invalid_url"
)

// Error normalizes backend-specific failures into Relurpify-level errors.
type Error struct {
	Code      ErrorCode
	Backend   string
	Operation string
	Err       error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	base := fmt.Sprintf("browser %s failed", e.Operation)
	if e.Backend != "" {
		base = fmt.Sprintf("%s (%s)", base, e.Backend)
	}
	if e.Code != "" {
		base = fmt.Sprintf("%s: %s", base, e.Code)
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", base, e.Err)
	}
	return base
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsErrorCode reports whether err contains a normalized browser error code.
func IsErrorCode(err error, code ErrorCode) bool {
	var browserErr *Error
	if !errors.As(err, &browserErr) {
		return false
	}
	return browserErr.Code == code
}

func wrapError(backend, operation string, err error) error {
	if err == nil {
		return nil
	}
	var browserErr *Error
	if errors.As(err, &browserErr) {
		if browserErr.Backend == "" {
			browserErr.Backend = backend
		}
		if browserErr.Operation == "" {
			browserErr.Operation = operation
		}
		return browserErr
	}
	return &Error{
		Code:      ErrUnknownOperation,
		Backend:   backend,
		Operation: operation,
		Err:       err,
	}
}
