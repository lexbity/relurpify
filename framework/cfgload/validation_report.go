package cfgload

import (
	"fmt"
	"strings"
)

// ValidationIssue describes a single validation failure with location and reason.
type ValidationIssue struct {
	File   string
	Field  string
	Value  any
	Reason string
}

// ValidationReport aggregates validation issues across multiple files.
type ValidationReport struct {
	Issues []ValidationIssue
}

// Add appends a validation issue to the report.
func (r *ValidationReport) Add(file, field string, value any, reason string) {
	if r == nil {
		return
	}
	r.Issues = append(r.Issues, ValidationIssue{
		File:   strings.TrimSpace(file),
		Field:  strings.TrimSpace(field),
		Value:  value,
		Reason: strings.TrimSpace(reason),
	})
}

// AddIssue appends an already constructed validation issue.
func (r *ValidationReport) AddIssue(issue ValidationIssue) {
	if r == nil {
		return
	}
	issue.File = strings.TrimSpace(issue.File)
	issue.Field = strings.TrimSpace(issue.Field)
	issue.Reason = strings.TrimSpace(issue.Reason)
	r.Issues = append(r.Issues, issue)
}

// Merge appends the issues from another report.
func (r *ValidationReport) Merge(other *ValidationReport) {
	if r == nil || other == nil || len(other.Issues) == 0 {
		return
	}
	r.Issues = append(r.Issues, other.Issues...)
}

// HasErrors reports whether the report contains validation failures.
func (r *ValidationReport) HasErrors() bool {
	return r != nil && len(r.Issues) > 0
}

// Error renders the report in a stable, human-readable format.
func (r *ValidationReport) Error() string {
	if r == nil || len(r.Issues) == 0 {
		return "config validation passed"
	}
	var b strings.Builder
	b.WriteString("config validation error:")
	for _, issue := range r.Issues {
		b.WriteString("\n  file:    ")
		b.WriteString(issue.File)
		b.WriteString("\n  field:   ")
		b.WriteString(issue.Field)
		b.WriteString("\n  value:   ")
		b.WriteString(formatValidationValue(issue.Value))
		b.WriteString("\n  reason:  ")
		b.WriteString(issue.Reason)
	}
	return b.String()
}

// String implements fmt.Stringer.
func (r *ValidationReport) String() string {
	return r.Error()
}

// Err returns nil for a clean report and the report itself when it contains errors.
func (r *ValidationReport) Err() error {
	if r == nil || len(r.Issues) == 0 {
		return nil
	}
	return r
}

func formatValidationValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "<nil>"
	case string:
		if v == "" {
			return `""`
		}
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", value)
	}
}
