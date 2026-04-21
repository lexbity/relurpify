package archaeoretrieval

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	frameworkretrieval "codeburg.org/lexbit/relurpify/framework/retrieval"
	_ "github.com/mattn/go-sqlite3"
)

func mustDB(t testing.TB) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "retrieval.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := frameworkretrieval.EnsureSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return db
}

func TestSQLStore_DeclareAndActiveAnchors(t *testing.T) {
	ctx := context.Background()
	store := NewSQLStore(mustDB(t))
	decl := frameworkretrieval.AnchorDeclaration{
		Term:       "verified",
		Definition: "reviewed by a human",
		Class:      "policy",
	}

	record, err := store.DeclareAnchor(ctx, decl, "scope", "trusted")
	if err != nil {
		t.Fatalf("DeclareAnchor: %v", err)
	}
	if record == nil {
		t.Fatal("expected anchor record")
	}

	anchors, err := store.ActiveAnchors(ctx, "scope")
	if err != nil {
		t.Fatalf("ActiveAnchors: %v", err)
	}
	if len(anchors) != 1 {
		t.Fatalf("len(anchors) = %d, want 1", len(anchors))
	}
	if anchors[0].AnchorID != record.AnchorID {
		t.Fatalf("anchor ID = %q, want %q", anchors[0].AnchorID, record.AnchorID)
	}
}

func TestSQLStore_InvalidateAnchor(t *testing.T) {
	ctx := context.Background()
	store := NewSQLStore(mustDB(t))
	record, err := store.DeclareAnchor(ctx, frameworkretrieval.AnchorDeclaration{
		Term:       "verified",
		Definition: "reviewed by a human",
		Class:      "policy",
	}, "scope", "trusted")
	if err != nil {
		t.Fatalf("DeclareAnchor: %v", err)
	}
	if err := frameworkretrieval.RecordAnchorDrift(ctx, SQLDB(store), record.AnchorID, "high", "stale implementation"); err != nil {
		t.Fatalf("RecordAnchorDrift: %v", err)
	}

	drifted, err := store.DriftedAnchors(ctx, "scope")
	if err != nil {
		t.Fatalf("DriftedAnchors before invalidate: %v", err)
	}
	if len(drifted) != 1 {
		t.Fatalf("len(drifted before invalidate) = %d, want 1", len(drifted))
	}

	if err := store.InvalidateAnchor(ctx, record.AnchorID, "stale"); err != nil {
		t.Fatalf("InvalidateAnchor: %v", err)
	}

	active, err := store.ActiveAnchors(ctx, "scope")
	if err != nil {
		t.Fatalf("ActiveAnchors: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("len(active) = %d, want 0", len(active))
	}

	drifted, err = store.DriftedAnchors(ctx, "scope")
	if err != nil {
		t.Fatalf("DriftedAnchors: %v", err)
	}
	if len(drifted) != 0 {
		t.Fatalf("len(drifted) = %d, want 0 after invalidation", len(drifted))
	}
}

func TestSQLStore_NilDB(t *testing.T) {
	var store *SQLStore = NewSQLStore(nil)
	ctx := context.Background()

	active, err := store.ActiveAnchors(ctx, "scope")
	if err != nil {
		t.Fatalf("ActiveAnchors: %v", err)
	}
	if active != nil {
		t.Fatalf("expected nil active anchors, got %#v", active)
	}

	drifted, err := store.DriftedAnchors(ctx, "scope")
	if err != nil {
		t.Fatalf("DriftedAnchors: %v", err)
	}
	if drifted != nil {
		t.Fatalf("expected nil drifted anchors, got %#v", drifted)
	}

	unresolved, err := store.UnresolvedDrifts(ctx, "scope")
	if err != nil {
		t.Fatalf("UnresolvedDrifts: %v", err)
	}
	if unresolved != nil {
		t.Fatalf("expected nil unresolved drifts, got %#v", unresolved)
	}

	record, err := store.DeclareAnchor(ctx, frameworkretrieval.AnchorDeclaration{}, "scope", "trusted")
	if err != nil {
		t.Fatalf("DeclareAnchor: %v", err)
	}
	if record != nil {
		t.Fatalf("expected nil record, got %#v", record)
	}

	if err := store.InvalidateAnchor(ctx, "anchor-1", "stale"); err != nil {
		t.Fatalf("InvalidateAnchor: %v", err)
	}
}

func TestSQLStore_SQLDB_Roundtrip(t *testing.T) {
	db := mustDB(t)
	if SQLDB(NewSQLStore(db)) != db {
		t.Fatal("expected SQLDB to return underlying db")
	}
	if SQLDB(nil) != nil {
		t.Fatal("expected SQLDB(nil) to be nil")
	}
}
