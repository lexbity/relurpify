package prompt

import "testing"

func TestValidateStructuredMapMissingRequiredKeys(t *testing.T) {
	issues := ValidateStructuredMap("prompt-1", "block-1", map[string]any{
		"present": true,
	}, []string{"present", "missing"})
	if len(issues) != 1 {
		t.Fatalf("expected 1 validation issue, got %d", len(issues))
	}
	if issues[0].Severity != SeverityError {
		t.Fatalf("expected error severity, got %v", issues[0].Severity)
	}
	if issues[0].Message != "missing required field: missing" {
		t.Fatalf("unexpected validation message: %q", issues[0].Message)
	}
}
