package identity

import "fmt"

// ResolutionErrorCode classifies identity resolution failures.
type ResolutionErrorCode string

const (
	ResolutionErrorBearerTokenRequired ResolutionErrorCode = "bearer_token_required"
	ResolutionErrorUnknownToken        ResolutionErrorCode = "unknown_token"
	ResolutionErrorTokenLookupFailed   ResolutionErrorCode = "token_lookup_failed"
	ResolutionErrorTokenExpired        ResolutionErrorCode = "token_expired"
	ResolutionErrorTokenRevoked        ResolutionErrorCode = "token_revoked"
	ResolutionErrorTenantLookupFailed  ResolutionErrorCode = "tenant_lookup_failed"
	ResolutionErrorSubjectLookupFailed ResolutionErrorCode = "subject_lookup_failed"
	ResolutionErrorDisabledTenant      ResolutionErrorCode = "disabled_tenant"
	ResolutionErrorDisabledSubject     ResolutionErrorCode = "disabled_subject"
	ResolutionErrorAmbiguousSubject    ResolutionErrorCode = "ambiguous_subject"
	ResolutionErrorInvalidPrincipal    ResolutionErrorCode = "invalid_principal"
)

// ResolutionError is the typed error returned by identity resolution code.
// Callers should inspect Code to distinguish credential problems from backend
// lookup failures.
type ResolutionError struct {
	Code    ResolutionErrorCode
	Message string
	Detail  map[string]any
	Err     error
}

func (e ResolutionError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return string(e.Code)
	}
	return "identity resolution error"
}

func (e ResolutionError) Unwrap() error {
	return e.Err
}

func (e ResolutionError) WithDetail(key string, value any) ResolutionError {
	if e.Detail == nil {
		e.Detail = map[string]any{}
	}
	e.Detail[key] = value
	return e
}

func NewResolutionError(code ResolutionErrorCode, message string) ResolutionError {
	return ResolutionError{Code: code, Message: message}
}

func WrapResolutionError(code ResolutionErrorCode, message string, err error) ResolutionError {
	return ResolutionError{Code: code, Message: message, Err: err}
}

func (e ResolutionError) Format(f fmt.State, verb rune) {
	switch verb {
	case 'v':
		if f.Flag('+') && e.Err != nil {
			_, _ = fmt.Fprintf(f, "%s: %v", e.Error(), e.Err)
			return
		}
		_, _ = fmt.Fprint(f, e.Error())
	case 's':
		_, _ = fmt.Fprint(f, e.Error())
	case 'q':
		_, _ = fmt.Fprintf(f, "%q", e.Error())
	}
}
