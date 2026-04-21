package fmp

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestJSONPackagerBuildSealAndUnseal(t *testing.T) {
	t.Parallel()

	packager := JSONPackager{
		RuntimeStore:   fakeWorkflowRuntimeStore{},
		KeyResolver:    testRecipientKeys(),
		LocalRecipient: "runtime://mesh-a/node-1/rt-1",
	}
	lineage := core.LineageRecord{
		LineageID:        "lineage-1",
		TenantID:         "tenant-1",
		TaskClass:        "agent.run",
		ContextClass:     "workflow-runtime",
		SensitivityClass: core.SensitivityClassModerate,
		Owner:            core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	attempt := core.AttemptRecord{
		AttemptID: "attempt-1",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-1",
		State:     core.AttemptStateRunning,
	}
	pkg, err := packager.BuildPackage(context.Background(), lineage, attempt, RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	if err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}
	sealed, err := packager.SealPackage(context.Background(), pkg.Manifest, pkg, []string{"runtime://mesh-a/node-1/rt-1"})
	if err != nil {
		t.Fatalf("SealPackage() error = %v", err)
	}
	var unsealed PortableContextPackage
	if err := packager.UnsealPackage(context.Background(), *sealed, &unsealed); err != nil {
		t.Fatalf("UnsealPackage() error = %v", err)
	}
	if string(unsealed.ExecutionPayload) != string(pkg.ExecutionPayload) {
		t.Fatalf("unsealed payload mismatch")
	}
	if string(sealed.CiphertextChunks[0]) == string(pkg.ExecutionPayload) {
		t.Fatal("sealed payload remained plaintext")
	}
}

func TestJSONPackagerRejectsUnknownRecipientOnUnseal(t *testing.T) {
	t.Parallel()

	packager := JSONPackager{
		RuntimeStore: fakeWorkflowRuntimeStore{},
		KeyResolver:  testRecipientKeys(),
	}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	attempt := core.AttemptRecord{
		AttemptID: "attempt-1",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-1",
		State:     core.AttemptStateRunning,
	}
	pkg, err := packager.BuildPackage(context.Background(), lineage, attempt, RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	if err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}
	sealed, err := packager.SealPackage(context.Background(), pkg.Manifest, pkg, []string{"runtime://mesh-a/node-1/rt-1"})
	if err != nil {
		t.Fatalf("SealPackage() error = %v", err)
	}
	var out PortableContextPackage
	err = (JSONPackager{
		RuntimeStore:   fakeWorkflowRuntimeStore{},
		KeyResolver:    testRecipientKeys(),
		LocalRecipient: "runtime://mesh-a/node-2/rt-9",
	}).UnsealPackage(context.Background(), *sealed, &out)
	if err == nil {
		t.Fatal("UnsealPackage() error = nil, want recipient failure")
	}
}

func TestJSONPackagerDetectsCiphertextTamper(t *testing.T) {
	t.Parallel()

	packager := JSONPackager{
		RuntimeStore:   fakeWorkflowRuntimeStore{},
		KeyResolver:    testRecipientKeys(),
		LocalRecipient: "runtime://mesh-a/node-1/rt-1",
	}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	attempt := core.AttemptRecord{
		AttemptID: "attempt-1",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-1",
		State:     core.AttemptStateRunning,
	}
	pkg, err := packager.BuildPackage(context.Background(), lineage, attempt, RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	if err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}
	sealed, err := packager.SealPackage(context.Background(), pkg.Manifest, pkg, []string{"runtime://mesh-a/node-1/rt-1"})
	if err != nil {
		t.Fatalf("SealPackage() error = %v", err)
	}
	sealed.CiphertextChunks[0][0] ^= 0xFF
	var out PortableContextPackage
	if err := packager.UnsealPackage(context.Background(), *sealed, &out); err == nil {
		t.Fatal("UnsealPackage() error = nil, want tamper failure")
	}
}

func TestJSONPackagerUsesChunkedTransferMode(t *testing.T) {
	t.Parallel()

	packager := JSONPackager{
		RuntimeStore:    fakeWorkflowRuntimeStore{},
		KeyResolver:     testRecipientKeys(),
		LocalRecipient:  "runtime://mesh-a/node-1/rt-1",
		InlineThreshold: 8,
		ChunkSize:       16,
	}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	attempt := core.AttemptRecord{
		AttemptID: "attempt-1",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-1",
		State:     core.AttemptStateRunning,
	}

	pkg, err := packager.BuildPackage(context.Background(), lineage, attempt, RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	if err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}
	if pkg.Manifest.TransferMode != core.TransferModeChunked {
		t.Fatalf("transfer mode = %s, want %s", pkg.Manifest.TransferMode, core.TransferModeChunked)
	}
	sealed, err := packager.SealPackage(context.Background(), pkg.Manifest, pkg, []string{"runtime://mesh-a/node-1/rt-1"})
	if err != nil {
		t.Fatalf("SealPackage() error = %v", err)
	}
	if len(sealed.CiphertextChunks) < 2 {
		t.Fatalf("chunk count = %d, want at least 2", len(sealed.CiphertextChunks))
	}
	var out PortableContextPackage
	if err := packager.UnsealPackage(context.Background(), *sealed, &out); err != nil {
		t.Fatalf("UnsealPackage() error = %v", err)
	}
	if string(out.ExecutionPayload) != string(pkg.ExecutionPayload) {
		t.Fatal("chunked payload mismatch")
	}
}

func TestJSONPackagerUsesEncryptedExternalObjectTransport(t *testing.T) {
	t.Parallel()

	store := &InMemoryEncryptedObjectStore{}
	packager := JSONPackager{
		RuntimeStore:      fakeWorkflowRuntimeStore{},
		KeyResolver:       testRecipientKeys(),
		ObjectStore:       store,
		LocalRecipient:    "runtime://mesh-a/node-1/rt-1",
		InlineThreshold:   8,
		ExternalThreshold: 16,
		ChunkSize:         12,
	}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	attempt := core.AttemptRecord{
		AttemptID: "attempt-1",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-1",
		State:     core.AttemptStateRunning,
	}

	pkg, err := packager.BuildPackage(context.Background(), lineage, attempt, RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	if err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}
	if pkg.Manifest.TransferMode != core.TransferModeExternal {
		t.Fatalf("transfer mode = %s, want %s", pkg.Manifest.TransferMode, core.TransferModeExternal)
	}
	sealed, err := packager.SealPackage(context.Background(), pkg.Manifest, pkg, []string{"runtime://mesh-a/node-1/rt-1"})
	if err != nil {
		t.Fatalf("SealPackage() error = %v", err)
	}
	if len(sealed.CiphertextChunks) != 0 {
		t.Fatalf("inline ciphertext chunks = %d, want 0 for external mode", len(sealed.CiphertextChunks))
	}
	if len(sealed.ExternalObjectRefs) == 0 {
		t.Fatal("external refs empty")
	}
	for _, ref := range sealed.ExternalObjectRefs {
		ciphertext, err := store.GetObject(context.Background(), ref)
		if err != nil {
			t.Fatalf("GetObject(%s) error = %v", ref, err)
		}
		if string(ciphertext) == string(pkg.ExecutionPayload) {
			t.Fatalf("external object %s remained plaintext", ref)
		}
	}
	var out PortableContextPackage
	if err := packager.UnsealPackage(context.Background(), *sealed, &out); err != nil {
		t.Fatalf("UnsealPackage() error = %v", err)
	}
	if string(out.ExecutionPayload) != string(pkg.ExecutionPayload) {
		t.Fatal("external payload mismatch")
	}
}

func TestJSONPackagerSupportsRecipientKeyRotation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	recipient := "runtime://mesh-a/node-1/rt-1"
	sourceTrust := &InMemoryTrustBundleStore{}
	if err := sourceTrust.UpsertTrustBundle(context.Background(), core.TrustBundle{
		TrustDomain: "mesh-a",
		BundleID:    "bundle-1",
		RecipientKeys: []core.RecipientKeyAdvertisement{
			{Recipient: recipient, KeyID: "old", Version: "v1", PublicKey: []byte("0123456789abcdef0123456789abcdef"), Active: true, ExpiresAt: now.Add(time.Hour)},
			{Recipient: recipient, KeyID: "new", Version: "v2", PublicKey: []byte("abcdef0123456789abcdef0123456789"), Active: true, ExpiresAt: now.Add(2 * time.Hour)},
		},
		IssuedAt:  now,
		ExpiresAt: now.Add(3 * time.Hour),
	}); err != nil {
		t.Fatalf("UpsertTrustBundle(source) error = %v", err)
	}
	packager := JSONPackager{
		RuntimeStore: fakeWorkflowRuntimeStore{},
		KeyResolver: &TrustBundleRecipientKeyResolver{
			Trust: sourceTrust,
			Now:   func() time.Time { return now },
		},
		LocalRecipient: recipient,
	}
	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	attempt := core.AttemptRecord{
		AttemptID: "attempt-1",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-1",
		State:     core.AttemptStateRunning,
	}
	pkg, err := packager.BuildPackage(context.Background(), lineage, attempt, RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	if err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}
	sealed, err := packager.SealPackage(context.Background(), pkg.Manifest, pkg, []string{recipient})
	if err != nil {
		t.Fatalf("SealPackage() error = %v", err)
	}
	wrappedKeys, err := parseWrappedKeys(sealed.ReplayProtectionData)
	if err != nil {
		t.Fatalf("parseWrappedKeys() error = %v", err)
	}
	if len(wrappedKeys[recipient]) != 2 {
		t.Fatalf("wrapped keys = %+v", wrappedKeys[recipient])
	}

	destTrust := &InMemoryTrustBundleStore{}
	if err := destTrust.UpsertTrustBundle(context.Background(), core.TrustBundle{
		TrustDomain: "mesh-a",
		BundleID:    "bundle-2",
		RecipientKeys: []core.RecipientKeyAdvertisement{
			{Recipient: recipient, KeyID: "new", Version: "v2", PublicKey: []byte("abcdef0123456789abcdef0123456789"), Active: true, ExpiresAt: now.Add(2 * time.Hour)},
		},
		IssuedAt:  now,
		ExpiresAt: now.Add(3 * time.Hour),
	}); err != nil {
		t.Fatalf("UpsertTrustBundle(dest) error = %v", err)
	}
	var out PortableContextPackage
	err = (JSONPackager{
		RuntimeStore: fakeWorkflowRuntimeStore{},
		KeyResolver: &TrustBundleRecipientKeyResolver{
			Trust: destTrust,
			Now:   func() time.Time { return now },
		},
		LocalRecipient: recipient,
	}).UnsealPackage(context.Background(), *sealed, &out)
	if err != nil {
		t.Fatalf("UnsealPackage() error = %v", err)
	}
	if string(out.ExecutionPayload) != string(pkg.ExecutionPayload) {
		t.Fatal("rotation payload mismatch")
	}
}
