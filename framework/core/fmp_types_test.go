package core

import (
	"testing"
	"time"
)

func TestLineageRecordValidate(t *testing.T) {
	t.Parallel()

	record := LineageRecord{
		LineageID:        "lineage-1",
		TenantID:         "tenant-1",
		TaskClass:        "agent.run",
		ContextClass:     "workflow-runtime",
		Owner:            SubjectRef{TenantID: "tenant-1", Kind: SubjectKindServiceAccount, ID: "svc-1"},
		SensitivityClass: SensitivityClassModerate,
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestLineageRecordValidateRejectsTenantMismatch(t *testing.T) {
	t.Parallel()

	record := LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        SubjectRef{TenantID: "tenant-2", Kind: SubjectKindServiceAccount, ID: "svc-1"},
	}
	if err := record.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want tenant mismatch")
	}
}

func TestLeaseTokenValidateRejectsExpiredWindow(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	token := LeaseToken{
		LeaseID:   "lease-1",
		LineageID: "lineage-1",
		AttemptID: "attempt-1",
		Issuer:    "issuer",
		IssuedAt:  now,
		Expiry:    now,
	}
	if err := token.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid expiry")
	}
}

func TestHandoffOfferValidateRequiresLease(t *testing.T) {
	t.Parallel()

	offer := HandoffOffer{
		OfferID:            "offer-1",
		LineageID:          "lineage-1",
		SourceAttemptID:    "attempt-1",
		SourceRuntimeID:    "rt-1",
		DestinationExport:  "exp.run",
		ContextManifestRef: "ctx-1",
		ContextClass:       "workflow-runtime",
		Expiry:             time.Now().UTC().Add(time.Minute),
	}
	if err := offer.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing lease")
	}
}
