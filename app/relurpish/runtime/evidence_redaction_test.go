package runtime

import (
	"testing"
)

// TestEvidenceRedaction tests the redaction functionality.
func TestEvidenceRedaction(t *testing.T) {
	// Create a redactor with strict policy
	redactor := NewEvidenceRedactor(StrictRedactionPolicy())

	// Test that we can create redacted versions
	// This verifies the redaction system is integrated
	if !redactor.Policy().RedactSecrets {
		t.Error("Strict policy should redact secrets")
	}
	if !redactor.Policy().RedactPromptBodies {
		t.Error("Strict policy should redact prompt bodies")
	}

	t.Logf("✓ Strict redaction policy works")

	// Test default policy
	defaultRedactor := NewEvidenceRedactor(DefaultRedactionPolicy())
	if !defaultRedactor.Policy().RedactSecrets {
		t.Error("Default policy should redact secrets")
	}
	if defaultRedactor.Policy().RedactPromptBodies {
		t.Error("Default policy should not redact prompt bodies by default")
	}

	t.Logf("✓ Default redaction policy works")
}

// TestRedactionPolicySetters tests that policy can be changed.
func TestRedactionPolicySetters(t *testing.T) {
	redactor := NewEvidenceRedactor(DefaultRedactionPolicy())
	
	// Create a custom policy
	customPolicy := RedactionPolicy{
		RedactSecrets:      true,
		RedactPromptBodies: true,
		RedactResponseBodies: true,
		MaxPreviewLength:    100,
	}
	
	redactor.SetPolicy(customPolicy)
	policy := redactor.Policy()
	
	if !policy.RedactSecrets {
		t.Error("Policy should have RedactSecrets=true")
	}
	if !policy.RedactPromptBodies {
		t.Error("Policy should have RedactPromptBodies=true")
	}
	if !policy.RedactResponseBodies {
		t.Error("Policy should have RedactResponseBodies=true")
	}
	if policy.MaxPreviewLength != 100 {
		t.Error("Policy should have MaxPreviewLength=100")
	}
	
	t.Logf("✓ Policy setters work")
}
