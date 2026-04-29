package fmp

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

type testRuntimeStore struct{}

func (testRuntimeStore) QueryWorkflowRuntime(context.Context, string, string) (map[string]any, error) {
	return map[string]any{"status": "ok"}, nil
}

func TestServiceOpenChunkTransferAndReadWithAck(t *testing.T) {
	t.Parallel()

	lineage := LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        identity.SubjectRef{TenantID: "tenant-1", Kind: identity.SubjectKindServiceAccount, ID: "svc-1"},
	}
	attempt := AttemptRecord{
		AttemptID: "attempt-1",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-1",
		State:     AttemptStateRunning,
		StartTime: time.Now().UTC(),
	}
	store := &InMemoryOwnershipStore{}
	if err := store.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	if err := store.UpsertAttempt(context.Background(), attempt); err != nil {
		t.Fatalf("UpsertAttempt() error = %v", err)
	}
	packager := JSONPackager{
		RuntimeStore:    testRuntimeStore{},
		KeyResolver:     StaticRecipientKeyResolver{"runtime://mesh-a/node-1/rt-1": []byte("0123456789abcdef")},
		LocalRecipient:  "runtime://mesh-a/node-1/rt-1",
		InlineThreshold: 8,
		ChunkSize:       16,
	}
	pkg, err := packager.BuildPackage(context.Background(), lineage, attempt, RuntimeQuery{WorkflowID: "wf-1", RunID: "run-1"})
	if err != nil {
		t.Fatalf("BuildPackage() error = %v", err)
	}
	sealed, err := packager.SealPackage(context.Background(), pkg.Manifest, pkg, []string{"runtime://mesh-a/node-1/rt-1"})
	if err != nil {
		t.Fatalf("SealPackage() error = %v", err)
	}

	svc := &Service{
		Ownership: store,
		Transfers: &InMemoryChunkTransferManager{DefaultWindow: 1},
	}
	session, err := svc.OpenChunkTransfer(context.Background(), lineage.LineageID, pkg.Manifest, *sealed)
	if err != nil {
		t.Fatalf("OpenChunkTransfer() error = %v", err)
	}
	if session.TotalChunks < 2 {
		t.Fatalf("TotalChunks = %d, want at least 2", session.TotalChunks)
	}
	first, control, err := svc.ReadChunkTransfer(context.Background(), session.TransferID, 2)
	if err != nil {
		t.Fatalf("ReadChunkTransfer(first) error = %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first chunk batch len = %d, want 1 due to transfer window", len(first))
	}
	if control.Backpressure {
		t.Fatal("unexpected backpressure on first read")
	}
	control, err = svc.AckChunkTransfer(context.Background(), lineage.LineageID, ChunkAck{
		TransferID: session.TransferID,
		AckedIndex: first[0].Index,
		WindowSize: 2,
	})
	if err != nil {
		t.Fatalf("AckChunkTransfer() error = %v", err)
	}
	if control.WindowSize != 2 {
		t.Fatalf("WindowSize = %d, want 2", control.WindowSize)
	}
	second, control, err := svc.ReadChunkTransfer(context.Background(), session.TransferID, 2)
	if err != nil {
		t.Fatalf("ReadChunkTransfer(second) error = %v", err)
	}
	if len(second) == 0 {
		t.Fatal("second chunk batch empty")
	}
	if control.Remaining < 0 {
		t.Fatalf("Remaining = %d, want >= 0", control.Remaining)
	}
}

func TestInMemoryChunkTransferManagerAppliesBackpressure(t *testing.T) {
	t.Parallel()

	manager := &InMemoryChunkTransferManager{DefaultWindow: 1}
	now := time.Now().UTC()
	manifest := ContextManifest{
		ContextID:      "ctx-1",
		LineageID:      "lineage-1",
		AttemptID:      "attempt-1",
		ContextClass:   "workflow-runtime",
		SchemaVersion:  "fmp.context.v1",
		ContentHash:    "abc123",
		TransferMode:   TransferModeChunked,
		EncryptionMode: EncryptionModeEndToEnd,
		ChunkCount:     2,
	}
	sealed := SealedContext{
		EnvelopeVersion:    "fmp.sealed.v1",
		ContextManifestRef: manifest.ContextID,
		CipherSuite:        "aes-gcm-256",
		CiphertextChunks:   [][]byte{[]byte("a"), []byte("b")},
		IntegrityTag:       "tag",
	}
	session, err := manager.Open(context.Background(), manifest, sealed, now)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	frames, control, err := manager.Read(context.Background(), session.TransferID, 2, now)
	if err != nil {
		t.Fatalf("Read(first) error = %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("len(first) = %d, want 1", len(frames))
	}
	frames, control, err = manager.Read(context.Background(), session.TransferID, 2, now)
	if err != nil {
		t.Fatalf("Read(second) error = %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("len(second) = %d, want 0 under backpressure", len(frames))
	}
	if control == nil || !control.Backpressure {
		t.Fatal("expected backpressure control")
	}
}

func TestServiceCancelChunkTransfer(t *testing.T) {
	t.Parallel()

	lineage := LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        identity.SubjectRef{TenantID: "tenant-1", Kind: identity.SubjectKindServiceAccount, ID: "svc-1"},
	}
	store := &InMemoryOwnershipStore{}
	if err := store.CreateLineage(context.Background(), lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	manager := &InMemoryChunkTransferManager{DefaultWindow: 1}
	manifest := ContextManifest{
		ContextID:      "ctx-1",
		LineageID:      lineage.LineageID,
		AttemptID:      "attempt-1",
		ContextClass:   "workflow-runtime",
		SchemaVersion:  "fmp.context.v1",
		ContentHash:    "abc123",
		TransferMode:   TransferModeChunked,
		EncryptionMode: EncryptionModeEndToEnd,
		ChunkCount:     1,
	}
	sealed := SealedContext{
		EnvelopeVersion:    "fmp.sealed.v1",
		ContextManifestRef: manifest.ContextID,
		CipherSuite:        "aes-gcm-256",
		CiphertextChunks:   [][]byte{[]byte("a")},
		IntegrityTag:       "tag",
	}
	svc := &Service{Ownership: store, Transfers: manager}
	session, err := svc.OpenChunkTransfer(context.Background(), lineage.LineageID, manifest, sealed)
	if err != nil {
		t.Fatalf("OpenChunkTransfer() error = %v", err)
	}
	if err := svc.CancelChunkTransfer(context.Background(), lineage.LineageID, session.TransferID, "receiver closed"); err != nil {
		t.Fatalf("CancelChunkTransfer() error = %v", err)
	}
	if _, _, err := svc.ReadChunkTransfer(context.Background(), session.TransferID, 1); err == nil {
		t.Fatal("ReadChunkTransfer() error = nil after cancel")
	}
}
