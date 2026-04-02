package modes

import (
	"testing"
)

func TestFindingsFromSemanticState_GroupsBySeverity(t *testing.T) {
	content := findingsFromSemanticState(map[string]any{
		"euclo.review_findings": map[string]any{
			"findings": []any{
				map[string]any{"severity": "critical", "location": "api.go:10", "description": "breaking API removal"},
				map[string]any{"severity": "warning", "location": "main.go:3", "description": "missing verification"},
				map[string]any{"severity": "info", "location": "", "description": "clean follow-up"},
			},
		},
	})

	if len(content.Critical) != 1 || len(content.Warning) != 1 || len(content.Info) != 1 {
		t.Fatalf("unexpected grouped findings: %#v", content)
	}
}

func TestApprovalStatusFromSemanticState_ReadsApprovalDecision(t *testing.T) {
	status := approvalStatusFromSemanticState(map[string]any{
		"euclo.review_findings": map[string]any{
			"approval_decision": map[string]any{"status": "blocked"},
		},
	})
	if status != "blocked" {
		t.Fatalf("expected blocked approval, got %q", status)
	}
}
