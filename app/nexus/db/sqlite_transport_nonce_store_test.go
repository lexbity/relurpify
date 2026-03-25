package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteTransportNonceStoreRejectsReplayAcrossReopen(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "transport_nonces.db")
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	store, err := NewSQLiteTransportNonceStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteTransportNonceStore() error = %v", err)
	}
	store.Now = func() time.Time { return now }
	if err := store.Reserve(context.Background(), "mesh.remote:gw-1", "nonce-1", now.Add(time.Minute)); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := NewSQLiteTransportNonceStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteTransportNonceStore(reopen) error = %v", err)
	}
	reopened.Now = func() time.Time { return now }
	defer reopened.Close()
	if err := reopened.Reserve(context.Background(), "mesh.remote:gw-1", "nonce-1", now.Add(time.Minute)); err == nil {
		t.Fatal("expected replay rejection after reopen")
	}
}

func TestSQLiteTransportNonceStoreExpiresEntries(t *testing.T) {
	t.Parallel()

	store, err := NewSQLiteTransportNonceStore(filepath.Join(t.TempDir(), "transport_nonces.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTransportNonceStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	store.Now = func() time.Time { return now }
	if err := store.Reserve(context.Background(), "mesh.remote:gw-1", "nonce-1", now.Add(time.Second)); err != nil {
		t.Fatalf("Reserve(first) error = %v", err)
	}
	store.Now = func() time.Time { return now.Add(2 * time.Second) }
	if err := store.Reserve(context.Background(), "mesh.remote:gw-1", "nonce-1", now.Add(3*time.Second)); err != nil {
		t.Fatalf("Reserve(after expiry) error = %v", err)
	}
}
