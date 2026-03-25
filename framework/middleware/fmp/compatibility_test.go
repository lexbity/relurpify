package fmp

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestValidateImportedContextCompatibilityRejectsUnsupportedCipherSuite(t *testing.T) {
	t.Parallel()

	err := ValidateImportedContextCompatibility(core.RuntimeDescriptor{
		RuntimeID:                 "rex",
		SupportedContextClasses:   []string{"workflow-runtime"},
		SupportedEncryptionSuites: []string{"suite-a"},
		MaxContextSize:            1024,
	}, core.ContextManifest{
		ContextClass:  "workflow-runtime",
		SchemaVersion: "fmp.context.v1",
		SizeBytes:     128,
	}, core.SealedContext{
		CipherSuite: "suite-b",
	})
	if err == nil {
		t.Fatal("ValidateImportedContextCompatibility() error = nil, want cipher suite rejection")
	}
}

func TestValidateOfferCompatibilityRejectsExpiredRuntime(t *testing.T) {
	t.Parallel()

	refusal := ValidateOfferCompatibility(core.RuntimeDescriptor{
		RuntimeID:               "rex",
		SupportedContextClasses: []string{"workflow-runtime"},
		CompatibilityClass:      "compat-a",
		ExpiresAt:               time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
	}, core.HandoffOffer{
		ContextClass:             "workflow-runtime",
		ContextSizeBytes:         128,
		SourceCompatibilityClass: "compat-a",
	}, core.ExportDescriptor{
		RequiredCompatibilityClasses: []string{"compat-a"},
	}, time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC))
	if refusal == nil || refusal.Code != core.RefusalAdmissionClosed {
		t.Fatalf("refusal = %+v", refusal)
	}
}
