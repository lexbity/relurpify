package prompt

import "errors"

// NotFoundError is returned when the requested prompt ID is not in the registry.
type NotFoundError struct {
	ID string
}

func (e *NotFoundError) Error() string {
	return "prompt not found: " + e.ID
}

// ParadigmMismatchError is returned when the consuming paradigm is not in the
// prompt's paradigm tag list (and the list is non-empty).
type ParadigmMismatchError struct {
	ID       string
	Required []string
	Actual   string
}

func (e *ParadigmMismatchError) Error() string {
	return "paradigm mismatch for prompt " + e.ID + ": requires one of " +
		joinStrings(e.Required, ", ") + ", got " + e.Actual
}

func joinStrings(ss []string, sep string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += sep
		}
		out += s
	}
	return out
}

// ValidationError wraps a set of validation issues as an error.
type ValidationError struct {
	Issues []ValidationIssue
}

func (e *ValidationError) Error() string {
	if len(e.Issues) == 1 {
		return e.Issues[0].Error()
	}
	return "prompt validation failed with " + itoa(len(e.Issues)) + " issues"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 10)
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// DuplicateIDError is returned when two prompt files declare the same ID.
type DuplicateIDError struct {
	ID           string
	ExistingPath string
	NewPath      string
}

func (e *DuplicateIDError) Error() string {
	if e == nil {
		return "duplicate prompt id"
	}
	if e.ExistingPath != "" && e.NewPath != "" {
		return "duplicate prompt id: " + e.ID + " (" + e.ExistingPath + ", " + e.NewPath + ")"
	}
	if e.NewPath != "" {
		return "duplicate prompt id: " + e.ID + " (" + e.NewPath + ")"
	}
	return "duplicate prompt id: " + e.ID
}

// UnknownVariableError is returned when a body references a variable that has
// no runtime value or declared default.
type UnknownVariableError struct {
	Name string
}

func (e *UnknownVariableError) Error() string {
	return "unknown variable: " + e.Name
}

// InvalidVariableReferenceError is returned when a body contains malformed
// brace substitution syntax.
type InvalidVariableReferenceError struct {
	Reference string
}

func (e *InvalidVariableReferenceError) Error() string {
	return "invalid variable reference: " + e.Reference
}

// alreadyRegisteredError is the sentinel used by the registry when a duplicate
// provider name is registered. Callers use IsAlreadyRegistered to detect it.
type alreadyRegisteredError struct {
	Name string
}

func (e *alreadyRegisteredError) Error() string {
	return "provider already registered: " + e.Name
}

func (e *alreadyRegisteredError) isAlreadyRegistered() {}

// alreadyRegisteredMarker is the interface implemented by alreadyRegisteredError
// and any compatible error returned by MockRegistry.
type alreadyRegisteredMarker interface {
	isAlreadyRegistered()
}

// ErrAlreadyRegistered returns the already-registered sentinel error for name.
// Use this in mock implementations (e.g. prompttest.MockRegistry) so that
// IsAlreadyRegistered correctly identifies the error.
func ErrAlreadyRegistered(name string) error {
	return &alreadyRegisteredError{Name: name}
}

// IsAlreadyRegistered reports whether err signals a duplicate provider name.
// RegisterAll implementations call this to skip already-registered shared providers.
func IsAlreadyRegistered(err error) bool {
	if err == nil {
		return false
	}
	var m alreadyRegisteredMarker
	return errors.As(err, &m)
}
