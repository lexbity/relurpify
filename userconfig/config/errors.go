package config

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorKind classifies loader and audit failures.
type ErrorKind string

const (
	// ErrKindSchema is returned when schema parsing, lookup, or body decoding
	// fails.
	ErrKindSchema ErrorKind = "schema"
)

var (
	// ErrMissingSchemaDeclaration reports that a file does not begin with a
	// required schema declaration.
	ErrMissingSchemaDeclaration = errors.New("missing schema declaration")
	// ErrInvalidSchemaDeclaration reports that a schema line exists but does not
	// match the relurpify schema format.
	ErrInvalidSchemaDeclaration = errors.New("invalid schema declaration")
	// ErrUnknownSchema reports a schema namespace that is not registered.
	ErrUnknownSchema = errors.New("unknown schema")
	// ErrUnsupportedSchemaVersion reports a known schema namespace with an
	// unsupported version number.
	ErrUnsupportedSchemaVersion = errors.New("unsupported schema version")
	// ErrYAMLAnchorAlias reports an anchor or alias node in a schema file body.
	ErrYAMLAnchorAlias = errors.New("yaml anchor or alias not allowed")
	// ErrForbiddenSecretField reports a forbidden secret-bearing field name in a config file.
	ErrForbiddenSecretField = errors.New("secret field detected in config file")
)

// SchemaError wraps schema parsing and body-decoding failures with location
// context.
type SchemaError struct {
	Path   string
	Line   int
	Schema string
	Key    string
	Err    error
}

func (e *SchemaError) Error() string {
	if e == nil {
		return "<nil>"
	}
	parts := make([]string, 0, 4)
	parts = append(parts, "schema error")
	if e.Path != "" {
		parts = append(parts, e.Path)
	}
	if e.Line > 0 {
		parts = append(parts, fmt.Sprintf("line %d", e.Line))
	}
	if e.Schema != "" {
		parts = append(parts, e.Schema)
	}
	if e.Key != "" {
		parts = append(parts, e.Key)
	}
	if e.Err != nil {
		parts = append(parts, e.Err.Error())
	}
	return strings.Join(parts, ": ")
}

func (e *SchemaError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// SecretFieldError reports a forbidden secret-bearing field in a config file.
type SecretFieldError struct {
	Path  string
	Field string
	Hint  string
}

func (e *SecretFieldError) Error() string {
	if e == nil {
		return "<nil>"
	}
	var b strings.Builder
	b.WriteString("FATAL config: ")
	b.WriteString(ErrForbiddenSecretField.Error())
	if e.Path != "" {
		b.WriteString("\n  file=")
		b.WriteString(e.Path)
	}
	if e.Field != "" {
		b.WriteString("\n  field=")
		b.WriteString(e.Field)
	}
	if e.Hint != "" {
		b.WriteString("\n  hint=")
		b.WriteString(e.Hint)
	}
	if e.Path != "" {
		b.WriteString("\n  See: docs/configuration/secrets.md")
	}
	return b.String()
}
