package config

import (
	"strings"
	"testing"
)

func TestValidationReportFormatsMultipleIssues(t *testing.T) {
	report := &ValidationReport{}
	report.Add("relurpify_cfg/workspace.yaml", "paths.state_dir", "/tmp", "must be relative")
	report.Add("relurpify_cfg/agent.yaml", "spec.security.run_as_user", 0, "must not be 0 (root)")

	got := report.Error()
	if !strings.Contains(got, "config validation error:") {
		t.Fatalf("report header missing: %s", got)
	}
	if !strings.Contains(got, "file:    relurpify_cfg/workspace.yaml") {
		t.Fatalf("workspace file missing from report: %s", got)
	}
	if !strings.Contains(got, "field:   spec.security.run_as_user") {
		t.Fatalf("agent field missing from report: %s", got)
	}
	if !strings.Contains(got, "value:   0") {
		t.Fatalf("value missing from report: %s", got)
	}
	if !strings.Contains(got, "reason:  must be relative") {
		t.Fatalf("reason missing from report: %s", got)
	}
}
