package main

import (
	"strings"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

type configCheck struct{}

func init() {
	registerCheck(configCheck{})
}

func (c configCheck) Name() string { return "config" }

func (c configCheck) Run(workspace string) []Diagnostic {
	report := config.ValidateWorkspaceTree(workspace)
	if !report.HasErrors() {
		return nil
	}
	var diags []Diagnostic
	for _, issue := range report.Issues {
		if isToolIssue(issue) {
			continue
		}
		code := "config.schema"
		if issue.File == "" {
			code = "config.generic"
		}
		diags = append(diags, Diagnostic{
			Check:    "config",
			Code:     code,
			Severity: SeverityError,
			Loc:      SourceLoc{File: issue.File, Line: extractLine(issue.Reason)},
			Message:  issue.Reason,
		})
	}
	return diags
}

// isToolIssue returns true if the validation issue relates to tool
// manifests or tool registry validation.
func isToolIssue(issue config.ValidationIssue) bool {
	if strings.HasSuffix(issue.File, ".tool.yaml") {
		return true
	}
	if issue.File == "relurpify_cfg/tools" {
		return true
	}
	return false
}

// extractLine attempts to extract a line number from a validation reason
// string (e.g. "yaml: line 42: ...").
func extractLine(reason string) int {
	const prefix = "line "
	idx := strings.Index(reason, prefix)
	if idx < 0 {
		return 0
	}
	rest := reason[idx+len(prefix):]
	end := strings.IndexAny(rest, ": \n")
	if end < 0 {
		return 0
	}
	var n int
	for _, c := range rest[:end] {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
