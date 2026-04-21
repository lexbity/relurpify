package fmp

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestSQLiteOwnershipStorePersistsCommitFencedSource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ownership.db")
	store, err := NewSQLiteOwnershipStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteOwnershipStore() error = %v", err)
	}

	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	if err := store.CreateLineage(ctx, lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	source := core.AttemptRecord{
		AttemptID: "attempt-a",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-a",
		State:     core.AttemptStateRunning,
		StartTime: time.Now().UTC(),
	}
	dest := core.AttemptRecord{
		AttemptID:         "attempt-b",
		LineageID:         lineage.LineageID,
		RuntimeID:         "rt-b",
		State:             core.AttemptStateResumePending,
		StartTime:         time.Now().UTC(),
		PreviousAttemptID: source.AttemptID,
	}
	if err := store.UpsertAttempt(ctx, source); err != nil {
		t.Fatalf("UpsertAttempt(source) error = %v", err)
	}
	if err := store.UpsertAttempt(ctx, dest); err != nil {
		t.Fatalf("UpsertAttempt(dest) error = %v", err)
	}
	lease, err := store.IssueLease(ctx, lineage.LineageID, source.AttemptID, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("IssueLease() error = %v", err)
	}
	commit := core.ResumeCommit{
		LineageID:            lineage.LineageID,
		OldAttemptID:         source.AttemptID,
		NewAttemptID:         dest.AttemptID,
		DestinationRuntimeID: dest.RuntimeID,
		ReceiptRef:           "receipt-1",
		CommitTime:           time.Now().UTC(),
	}
	if err := store.CommitHandoff(ctx, commit); err != nil {
		t.Fatalf("CommitHandoff() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := NewSQLiteOwnershipStore(path)
	if err != nil {
		t.Fatalf("reopen NewSQLiteOwnershipStore() error = %v", err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Fatalf("reopened Close() error = %v", err)
		}
	}()

	gotLineage, ok, err := reopened.GetLineage(ctx, lineage.LineageID)
	if err != nil || !ok {
		t.Fatalf("GetLineage() error = %v ok=%v", err, ok)
	}
	if gotLineage.CurrentOwnerAttempt != dest.AttemptID || gotLineage.CurrentOwnerRuntime != dest.RuntimeID {
		t.Fatalf("lineage owner mismatch: got attempt=%s runtime=%s", gotLineage.CurrentOwnerAttempt, gotLineage.CurrentOwnerRuntime)
	}
	if gotLineage.LineageVersion != 1 {
		t.Fatalf("lineage version = %d, want 1", gotLineage.LineageVersion)
	}
	gotDest, ok, err := reopened.GetAttempt(ctx, dest.AttemptID)
	if err != nil || !ok {
		t.Fatalf("GetAttempt(dest) error = %v ok=%v", err, ok)
	}
	if gotDest.State != core.AttemptStateRunning {
		t.Fatalf("dest state = %s, want %s", gotDest.State, core.AttemptStateRunning)
	}
	gotSource, ok, err := reopened.GetAttempt(ctx, source.AttemptID)
	if err != nil || !ok {
		t.Fatalf("GetAttempt(source) error = %v ok=%v", err, ok)
	}
	if !gotSource.Fenced || gotSource.State != core.AttemptStateCommittedRemote || gotSource.FencingEpoch != lease.FencingEpoch {
		t.Fatalf("source attempt = %+v", *gotSource)
	}
}

func TestSQLiteOwnershipStoreSupersedesLeaseAcrossRestart(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ownership.db")
	store, err := NewSQLiteOwnershipStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteOwnershipStore() error = %v", err)
	}

	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	attempt := core.AttemptRecord{
		AttemptID: "attempt-a",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-a",
		State:     core.AttemptStateRunning,
		StartTime: time.Now().UTC(),
	}
	if err := store.CreateLineage(ctx, lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	if err := store.UpsertAttempt(ctx, attempt); err != nil {
		t.Fatalf("UpsertAttempt() error = %v", err)
	}
	firstLease, err := store.IssueLease(ctx, lineage.LineageID, attempt.AttemptID, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("IssueLease(first) error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := NewSQLiteOwnershipStore(path)
	if err != nil {
		t.Fatalf("reopen NewSQLiteOwnershipStore() error = %v", err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Fatalf("reopened Close() error = %v", err)
		}
	}()

	secondLease, err := reopened.IssueLease(ctx, lineage.LineageID, attempt.AttemptID, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("IssueLease(second) error = %v", err)
	}
	if secondLease.FencingEpoch <= firstLease.FencingEpoch {
		t.Fatalf("second fencing epoch = %d, want > %d", secondLease.FencingEpoch, firstLease.FencingEpoch)
	}
	if err := reopened.ValidateLease(ctx, *firstLease, time.Now().UTC()); err == nil {
		t.Fatal("ValidateLease(first) succeeded after superseding lease")
	}
	if err := reopened.ValidateLease(ctx, *secondLease, time.Now().UTC()); err != nil {
		t.Fatalf("ValidateLease(second) error = %v", err)
	}
}

func TestSQLiteOwnershipStoreCommitIsIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ownership.db")
	store, err := NewSQLiteOwnershipStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteOwnershipStore() error = %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}()

	lineage := core.LineageRecord{
		LineageID:    "lineage-1",
		TenantID:     "tenant-1",
		TaskClass:    "agent.run",
		ContextClass: "workflow-runtime",
		Owner:        core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindServiceAccount, ID: "svc-1"},
	}
	source := core.AttemptRecord{
		AttemptID: "attempt-a",
		LineageID: lineage.LineageID,
		RuntimeID: "rt-a",
		State:     core.AttemptStateRunning,
		StartTime: time.Now().UTC(),
	}
	dest := core.AttemptRecord{
		AttemptID:         "attempt-b",
		LineageID:         lineage.LineageID,
		RuntimeID:         "rt-b",
		State:             core.AttemptStateResumePending,
		StartTime:         time.Now().UTC(),
		PreviousAttemptID: source.AttemptID,
	}
	if err := store.CreateLineage(ctx, lineage); err != nil {
		t.Fatalf("CreateLineage() error = %v", err)
	}
	if err := store.UpsertAttempt(ctx, source); err != nil {
		t.Fatalf("UpsertAttempt(source) error = %v", err)
	}
	if err := store.UpsertAttempt(ctx, dest); err != nil {
		t.Fatalf("UpsertAttempt(dest) error = %v", err)
	}
	commit := core.ResumeCommit{
		LineageID:            lineage.LineageID,
		OldAttemptID:         source.AttemptID,
		NewAttemptID:         dest.AttemptID,
		DestinationRuntimeID: dest.RuntimeID,
		ReceiptRef:           "receipt-1",
		CommitTime:           time.Now().UTC(),
	}
	if err := store.CommitHandoff(ctx, commit); err != nil {
		t.Fatalf("CommitHandoff(first) error = %v", err)
	}
	if err := store.CommitHandoff(ctx, commit); err != nil {
		t.Fatalf("CommitHandoff(second) error = %v", err)
	}
	gotLineage, ok, err := store.GetLineage(ctx, lineage.LineageID)
	if err != nil || !ok {
		t.Fatalf("GetLineage() error = %v ok=%v", err, ok)
	}
	if gotLineage.LineageVersion != 1 {
		t.Fatalf("lineage version = %d, want 1 after idempotent retry", gotLineage.LineageVersion)
	}
}

func TestSQLiteOwnershipStoreReserveHandoffOfferPersistsAcrossRestart(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ownership.db")
	store, err := NewSQLiteOwnershipStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteOwnershipStore() error = %v", err)
	}
	reservation := HandoffOfferReservation{
		LineageID:            "lineage-1",
		OfferID:              "offer-1",
		LeaseID:              "lease-1",
		SourceAttemptID:      "attempt-a",
		FencingEpoch:         3,
		ProvisionalAttemptID: "lineage-1:rt-b:resume",
		RuntimeID:            "rt-b",
		CreatedAt:            time.Now().UTC(),
	}
	got, created, err := store.ReserveHandoffOffer(ctx, reservation)
	if err != nil {
		t.Fatalf("ReserveHandoffOffer(first) error = %v", err)
	}
	if !created || got == nil || got.RuntimeID != "rt-b" {
		t.Fatalf("first reserve = %+v created=%v", got, created)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := NewSQLiteOwnershipStore(path)
	if err != nil {
		t.Fatalf("reopen NewSQLiteOwnershipStore() error = %v", err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Fatalf("reopened Close() error = %v", err)
		}
	}()

	got, created, err = reopened.ReserveHandoffOffer(ctx, HandoffOfferReservation{
		LineageID:            "lineage-1",
		OfferID:              "offer-1",
		LeaseID:              "lease-1",
		SourceAttemptID:      "attempt-a",
		FencingEpoch:         3,
		ProvisionalAttemptID: "lineage-1:rt-c:resume",
		RuntimeID:            "rt-c",
		CreatedAt:            time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("ReserveHandoffOffer(second) error = %v", err)
	}
	if created {
		t.Fatal("expected existing reservation after restart")
	}
	if got == nil || got.RuntimeID != "rt-b" {
		t.Fatalf("existing reservation = %+v, want rt-b", got)
	}
}
