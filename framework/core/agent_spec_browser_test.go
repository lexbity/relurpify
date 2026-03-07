package core

import "testing"

func TestAgentBrowserSpecValidateRejectsUnknownBackend(t *testing.T) {
	spec := &AgentBrowserSpec{DefaultBackend: "playwright"}

	err := spec.Validate()

	if err == nil {
		t.Fatalf("expected validation error for unknown backend")
	}
}

func TestAgentBrowserSpecValidateRejectsUnknownAction(t *testing.T) {
	spec := &AgentBrowserSpec{
		Actions: map[string]AgentPermissionLevel{
			"teleport": AgentPermissionAllow,
		},
	}

	err := spec.Validate()

	if err == nil {
		t.Fatalf("expected validation error for unknown action")
	}
}

func TestAgentBrowserSpecValidateAcceptsKnownValues(t *testing.T) {
	spec := &AgentBrowserSpec{
		Enabled:         true,
		DefaultBackend:  "cdp",
		AllowedBackends: []string{"cdp", "bidi"},
		Actions: map[string]AgentPermissionLevel{
			"navigate":   AgentPermissionAllow,
			"execute_js": AgentPermissionAsk,
			"get_html":   AgentPermissionDeny,
		},
		Extraction: AgentBrowserExtractionSpec{
			DefaultMode:       "accessibility_plus_structured",
			MaxHTMLTokens:     4000,
			MaxSnapshotTokens: 1200,
		},
	}

	if err := spec.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
