package core

import (
	"testing"
	"time"
)

func TestFMPTypesValidationCoverage(t *testing.T) {
	now := time.Now().UTC()
	later := now.Add(time.Hour)
	validSubject := SubjectRef{TenantID: "tenant", Kind: SubjectKindUser, ID: "subject"}
	validEnvelope := CapabilityEnvelope{
		AllowedCapabilityIDs: []string{"cap-a"},
		AllowedTaskClasses:   []string{"task-a"},
	}
	validLease := LeaseToken{
		LeaseID:   "lease-1",
		LineageID: "lineage-1",
		AttemptID: "attempt-1",
		Issuer:    "issuer",
		IssuedAt:  now,
		Expiry:    later,
	}

	for _, tc := range []struct {
		name string
		err  error
	}{
		{"sensitivity", SensitivityClassHigh.Validate()},
		{"route", RouteModeGateway.Validate()},
		{"encryption", EncryptionModeEndToEnd.Validate()},
		{"transfer", TransferModeChunked.Validate()},
		{"refusal", RefusalUnauthorized.Validate()},
		{"attempt", AttemptStateCompleted.Validate()},
		{"receipt", ReceiptStatusRunning.Validate()},
	} {
		if tc.err != nil {
			t.Fatalf("%s validate: %v", tc.name, tc.err)
		}
	}

	if err := (SensitivityClass("bad")).Validate(); err == nil {
		t.Fatal("expected invalid sensitivity class error")
	}
	if err := (RouteMode("bad")).Validate(); err == nil {
		t.Fatal("expected invalid route mode error")
	}
	if err := (EncryptionMode("bad")).Validate(); err == nil {
		t.Fatal("expected invalid encryption mode error")
	}
	if err := (TransferMode("bad")).Validate(); err == nil {
		t.Fatal("expected invalid transfer mode error")
	}
	if err := (RefusalReasonCode("bad")).Validate(); err == nil {
		t.Fatal("expected invalid refusal code error")
	}
	if err := (AttemptState("bad")).Validate(); err == nil {
		t.Fatal("expected invalid attempt state error")
	}
	if err := (ReceiptStatus("bad")).Validate(); err == nil {
		t.Fatal("expected invalid receipt status error")
	}

	if err := (CapabilityEnvelope{MaxCPU: -1}).Validate(); err == nil {
		t.Fatal("expected capability envelope limit error")
	}
	if err := (CapabilityEnvelope{AllowedCapabilityIDs: []string{"ok", ""}}).Validate(); err == nil {
		t.Fatal("expected capability envelope empty value error")
	}
	if err := (TenantFederationPolicy{}).Validate(); err == nil {
		t.Fatal("expected tenant federation policy error")
	}
	if err := (TenantFederationPolicy{TenantID: "tenant", AllowedTrustDomains: []string{""}}).Validate(); err == nil {
		t.Fatal("expected tenant federation trust domain error")
	}
	if err := (TenantFederationPolicy{TenantID: "tenant", AllowedRouteModes: []RouteMode{RouteMode("bad")}}).Validate(); err == nil {
		t.Fatal("expected tenant federation route mode error")
	}
	if err := (TenantFederationPolicy{TenantID: "tenant", MaxTransferBytes: -1}).Validate(); err == nil {
		t.Fatal("expected tenant federation transfer limit error")
	}
	if err := (RuntimeDescriptor{}).Validate(); err == nil {
		t.Fatal("expected runtime descriptor error")
	}
	if err := (RuntimeDescriptor{RuntimeID: "rt", NodeID: "node", RuntimeVersion: "v", MaxContextSize: -1}).Validate(); err == nil {
		t.Fatal("expected runtime descriptor limits error")
	}
	if err := (ExportDescriptor{}).Validate(); err == nil {
		t.Fatal("expected export descriptor error")
	}
	if err := (ExportDescriptor{ExportName: "export", RouteMode: RouteModeGateway, SensitivityLimit: SensitivityClassHigh, AllowedCapabilityIDs: []string{"cap"}, AllowedTaskClasses: []string{"task"}, AcceptedIdentities: []SubjectRef{validSubject}}).Validate(); err != nil {
		t.Fatalf("unexpected valid export descriptor error: %v", err)
	}
	if err := (LineageRecord{}).Validate(); err == nil {
		t.Fatal("expected lineage record error")
	}
	if err := (LineageRecord{
		LineageID:          "lineage",
		TenantID:           "tenant",
		TaskClass:          "task",
		ContextClass:       "context",
		Owner:              validSubject,
		CapabilityEnvelope: validEnvelope,
		SensitivityClass:   SensitivityClassLow,
	}).Validate(); err != nil {
		t.Fatalf("unexpected valid lineage record error: %v", err)
	}
	if err := (AttemptRecord{}).Validate(); err == nil {
		t.Fatal("expected attempt record error")
	}
	if err := (AttemptRecord{
		AttemptID:   "attempt",
		LineageID:   "lineage",
		RuntimeID:   "runtime",
		State:       AttemptStateRunning,
		LeaseExpiry: later,
		StartTime:   now,
	}).Validate(); err != nil {
		t.Fatalf("unexpected valid attempt record error: %v", err)
	}
	if err := (TrustBundle{}).Validate(); err == nil {
		t.Fatal("expected trust bundle error")
	}
	if err := (TrustBundle{
		TrustDomain:       "trust",
		BundleID:          "bundle",
		GatewayIdentities: []SubjectRef{validSubject},
		TrustAnchors:      []string{"anchor"},
		RecipientKeys:     []RecipientKeyAdvertisement{{Recipient: "recipient", PublicKey: []byte{1}}},
		IssuedAt:          now,
		ExpiresAt:         later,
	}).Validate(); err != nil {
		t.Fatalf("unexpected valid trust bundle error: %v", err)
	}
	if err := (BoundaryPolicy{}).Validate(); err == nil {
		t.Fatal("expected boundary policy error")
	}
	if err := (BoundaryPolicy{
		TrustDomain:              "trust",
		AllowedRouteModes:        []RouteMode{RouteModeDirect},
		AcceptedSourceDomains:    []string{"source"},
		AcceptedSourceIdentities: []SubjectRef{validSubject},
	}).Validate(); err != nil {
		t.Fatalf("unexpected valid boundary policy error: %v", err)
	}
	if err := (GatewayForwardRequest{}).Validate(); err == nil {
		t.Fatal("expected gateway forward request error")
	}
	if err := (GatewayForwardRequest{
		TenantID:           "tenant",
		TrustDomain:        "trust",
		SourceDomain:       "source",
		GatewayIdentity:    validSubject,
		DestinationExport:  "export",
		RouteMode:          RouteModeGateway,
		ContextManifestRef: "manifest",
		SealedContext:      SealedContext{EnvelopeVersion: "v1", ContextManifestRef: "manifest", CipherSuite: "suite", IntegrityTag: "tag", CiphertextChunks: [][]byte{{1}}},
	}).Validate(); err != nil {
		t.Fatalf("unexpected valid gateway forward request error: %v", err)
	}
	if err := (GatewayForwardResult{}).Validate(); err == nil {
		t.Fatal("expected gateway forward result error")
	}
	if err := (GatewayForwardResult{TrustDomain: "trust", DestinationExport: "export", RouteMode: RouteModeDirect}).Validate(); err != nil {
		t.Fatalf("unexpected valid gateway forward result error: %v", err)
	}
	if err := (ContextManifest{}).Validate(); err == nil {
		t.Fatal("expected context manifest error")
	}
	if err := (ContextManifest{
		ContextID:        "ctx",
		LineageID:        "lineage",
		AttemptID:        "attempt",
		ContextClass:     "class",
		SchemaVersion:    "v1",
		ContentHash:      "hash",
		SensitivityClass: SensitivityClassLow,
		TransferMode:     TransferModeInline,
		EncryptionMode:   EncryptionModeLinkOnly,
	}).Validate(); err != nil {
		t.Fatalf("unexpected valid context manifest error: %v", err)
	}
	if err := (SealedContext{}).Validate(); err == nil {
		t.Fatal("expected sealed context error")
	}
	if err := (SealedContext{EnvelopeVersion: "v1", ContextManifestRef: "manifest", CipherSuite: "suite", IntegrityTag: "tag", CiphertextChunks: [][]byte{{1}}}).Validate(); err != nil {
		t.Fatalf("unexpected valid sealed context error: %v", err)
	}
	if err := (LeaseToken{}).Validate(); err == nil {
		t.Fatal("expected lease token error")
	}
	if err := validLease.Validate(); err != nil {
		t.Fatalf("unexpected valid lease token error: %v", err)
	}
	if err := (HandoffOffer{}).Validate(); err == nil {
		t.Fatal("expected handoff offer error")
	}
	if err := (HandoffOffer{
		OfferID:                       "offer",
		LineageID:                     "lineage",
		SourceAttemptID:               "attempt",
		SourceRuntimeID:               "runtime",
		DestinationExport:             "export",
		ContextManifestRef:            "manifest",
		ContextClass:                  "class",
		SensitivityClass:              SensitivityClassLow,
		RequestedCapabilityProjection: validEnvelope,
		LeaseToken:                    validLease,
		Expiry:                        later,
	}).Validate(); err != nil {
		t.Fatalf("unexpected valid handoff offer error: %v", err)
	}
	if err := (HandoffAccept{}).Validate(); err == nil {
		t.Fatal("expected handoff accept error")
	}
	if err := (HandoffAccept{
		OfferID:              "offer",
		DestinationRuntimeID: "runtime",
		AcceptedContextClass: "class",
		ProvisionalAttemptID: "attempt",
		Expiry:               later,
	}).Validate(); err != nil {
		t.Fatalf("unexpected valid handoff accept error: %v", err)
	}
	if err := (ResumeCommit{}).Validate(); err == nil {
		t.Fatal("expected resume commit error")
	}
	if err := (ResumeCommit{
		LineageID:            "lineage",
		OldAttemptID:         "old",
		NewAttemptID:         "new",
		DestinationRuntimeID: "runtime",
		ReceiptRef:           "receipt",
		CommitTime:           now,
	}).Validate(); err != nil {
		t.Fatalf("unexpected valid resume commit error: %v", err)
	}
	if err := (FenceNotice{}).Validate(); err == nil {
		t.Fatal("expected fence notice error")
	}
	if err := (FenceNotice{LineageID: "lineage", AttemptID: "attempt", Issuer: "issuer", FencingEpoch: 1}).Validate(); err != nil {
		t.Fatalf("unexpected valid fence notice error: %v", err)
	}
	if err := (ResumeReceipt{}).Validate(); err == nil {
		t.Fatal("expected resume receipt error")
	}
	if err := (ResumeReceipt{
		ReceiptID:         "receipt",
		LineageID:         "lineage",
		AttemptID:         "attempt",
		RuntimeID:         "runtime",
		ImportedContextID: "ctx",
		Status:            ReceiptStatusRunning,
	}).Validate(); err != nil {
		t.Fatalf("unexpected valid resume receipt error: %v", err)
	}
	if err := (TransferRefusal{}).Validate(); err == nil {
		t.Fatal("expected transfer refusal error")
	}
	if err := (TransferRefusal{Code: RefusalUnauthorized}).Validate(); err != nil {
		t.Fatalf("unexpected valid transfer refusal error: %v", err)
	}
	if err := (NodeAdvertisement{}).Validate(); err == nil {
		t.Fatal("expected node advertisement error")
	}
	if err := (NodeAdvertisement{TrustDomain: "trust", Node: NodeDescriptor{ID: "node", Name: "node", Platform: NodePlatformLinux, TenantID: "tenant", TrustClass: TrustClassRemoteApproved}}).Validate(); err != nil {
		t.Fatalf("unexpected valid node advertisement error: %v", err)
	}
	if err := (RuntimeAdvertisement{}).Validate(); err == nil {
		t.Fatal("expected runtime advertisement error")
	}
	if err := (RuntimeAdvertisement{TrustDomain: "trust", Runtime: RuntimeDescriptor{RuntimeID: "rt", NodeID: "node", RuntimeVersion: "v"}}).Validate(); err != nil {
		t.Fatalf("unexpected valid runtime advertisement error: %v", err)
	}
	if err := (ExportAdvertisement{}).Validate(); err == nil {
		t.Fatal("expected export advertisement error")
	}
	if err := (ExportAdvertisement{TrustDomain: "trust", Export: ExportDescriptor{ExportName: "export", RouteMode: RouteModeDirect, SensitivityLimit: SensitivityClassLow}}).Validate(); err != nil {
		t.Fatalf("unexpected valid export advertisement error: %v", err)
	}
	if err := (MessageEnvelope{}).Validate(); err == nil {
		t.Fatal("expected message envelope error")
	}
	if err := (MessageEnvelope{ProtocolVersion: "v1", MessageType: "notice"}).Validate(); err != nil {
		t.Fatalf("unexpected valid message envelope error: %v", err)
	}
}
