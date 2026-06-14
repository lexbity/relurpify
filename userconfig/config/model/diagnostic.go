package model

import "strings"

// LoadDiagnostic captures a file-level load issue from the model config loaders.
type LoadDiagnostic struct {
	Path     string
	Severity string
	Message  string
}

// HasBlockingDiagnostics reports whether any diagnostics are marked blocking.
func HasBlockingDiagnostics(diags []LoadDiagnostic) bool {
	for _, diag := range diags {
		if strings.EqualFold(strings.TrimSpace(diag.Severity), "blocking") {
			return true
		}
	}
	return false
}
