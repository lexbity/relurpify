package tui

import "testing"

func TestServiceSummaryLinesIncludeProvenance(t *testing.T) {
	lines := serviceSummaryLines(ServiceInfo{
		ID:     "browser",
		Status: ServiceStatusRunning,
		Source: "ayenitd/browser_service.go",
		Owner:  "workspace",
		Notes:  []string{"registered by ayenitd", "browser.enabled=true"},
	})
	if len(lines) < 4 {
		t.Fatalf("expected provenance lines, got %#v", lines)
	}
}
