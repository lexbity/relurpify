package retrieval

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupAnchorTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	ctx := context.Background()
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("failed to ensure schema: %v", err)
	}

	// Seed some test data
	docID := "test-doc-1"
	canonical := "/test/document.md"
	hash := "abc123def456"

	_, err = db.ExecContext(ctx, `
		INSERT INTO retrieval_documents
		(doc_id, canonical_uri, content_hash, corpus_scope, source_type, source_updated_at, last_ingested_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, docID, canonical, hash, "workspace", "markdown",
		time.Now().UTC().Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("failed to seed document: %v", err)
	}

	return db
}

func Test_AnchorCreation(t *testing.T) {
	db := setupAnchorTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create a basic anchor
	decl := AnchorDeclaration{
		Term:       "verified",
		Definition: "human-reviewed and approved by a domain expert",
		Class:      "policy",
		Context: map[string]string{
			"domain": "security",
		},
	}

	anchors := []AnchorDeclaration{decl}
	doc := DocumentRecord{
		DocID:       "test-doc-1",
		CorpusScope: "workspace",
	}
	chunks := []ChunkRecord{}
	versionID := "v1"

	err := extractAndPersistAnchors(ctx, db, anchors, versionID, chunks, doc)
	if err != nil {
		t.Fatalf("failed to extract and persist anchors: %v", err)
	}

	// Verify the anchor was created
	records, err := ActiveAnchors(ctx, db, "workspace")
	if err != nil {
		t.Fatalf("failed to query active anchors: %v", err)
	}

	if len(records) != 1 {
		t.Errorf("expected 1 anchor, got %d", len(records))
	}

	if records[0].Term != "verified" {
		t.Errorf("expected term 'verified', got '%s'", records[0].Term)
	}

	if records[0].Definition != "human-reviewed and approved by a domain expert" {
		t.Errorf("expected matching definition")
	}

	if records[0].AnchorClass != "policy" {
		t.Errorf("expected class 'policy', got '%s'", records[0].AnchorClass)
	}
}

func Test_AnchorTermNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Verified", "verified"},
		{"  VERIFIED  ", "verified"},
		{"  verified  test  ", "verified test"},
		{"Verified-Test", "verified-test"},
	}

	for _, tt := range tests {
		got := normalizeTerm(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeTerm(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func Test_AnchorSupersession(t *testing.T) {
	db := setupAnchorTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create initial anchor
	decl := AnchorDeclaration{
		Term:       "verified",
		Definition: "old definition",
		Class:      "policy",
	}

	anchors := []AnchorDeclaration{decl}
	doc := DocumentRecord{
		DocID:       "test-doc-1",
		CorpusScope: "workspace",
	}
	chunks := []ChunkRecord{}
	versionID := "v1"

	err := extractAndPersistAnchors(ctx, db, anchors, versionID, chunks, doc)
	if err != nil {
		t.Fatalf("failed to create initial anchor: %v", err)
	}

	// Get the initial anchor ID
	initial, err := ActiveAnchors(ctx, db, "workspace")
	if err != nil {
		t.Fatalf("failed to query anchors: %v", err)
	}
	if len(initial) != 1 {
		t.Fatalf("expected 1 anchor, got %d", len(initial))
	}
	initialID := initial[0].AnchorID

	// Supersede it with a new definition
	newDef := "new definition"
	newRecord, err := SupersedeAnchor(ctx, db, initialID, newDef, map[string]string{"updated": "true"})
	if err != nil {
		t.Fatalf("failed to supersede anchor: %v", err)
	}

	if newRecord.Definition != newDef {
		t.Errorf("expected definition %q, got %q", newDef, newRecord.Definition)
	}

	// Verify supersession link
	if newRecord.SupersededBy != nil {
		t.Errorf("expected new anchor to have empty superseded_by, got %q", *newRecord.SupersededBy)
	}

	// Get the old anchor and verify supersession link
	oldRecord, err := AnchorHistory(ctx, db, "verified", "workspace")
	if err != nil {
		t.Fatalf("failed to get anchor history: %v", err)
	}
	if len(oldRecord) == 0 {
		t.Fatalf("expected anchor history to be non-empty")
	}

	// Find the old anchor in the history
	var found bool
	for _, r := range oldRecord {
		if r.AnchorID == initialID {
			if r.SupersededBy == nil || *r.SupersededBy != newRecord.AnchorID {
				var got string
				if r.SupersededBy != nil {
					got = *r.SupersededBy
				}
				t.Errorf("expected old anchor SupersededBy=%q, got %q", newRecord.AnchorID, got)
			}
			found = true
			break
		}
	}
	if !found {
		t.Errorf("original anchor not found in history")
	}
}

func Test_AnchorDeduplication(t *testing.T) {
	db := setupAnchorTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create two anchor declarations with the same term and definition
	decl := AnchorDeclaration{
		Term:       "verified",
		Definition: "same definition",
		Class:      "policy",
	}

	doc := DocumentRecord{
		DocID:       "test-doc-1",
		CorpusScope: "workspace",
	}
	chunks := []ChunkRecord{}
	versionID := "v1"

	// Ingest first set of anchors
	err := extractAndPersistAnchors(ctx, db, []AnchorDeclaration{decl}, versionID, chunks, doc)
	if err != nil {
		t.Fatalf("failed to create first anchor: %v", err)
	}

	// Ingest the same anchor again
	err = extractAndPersistAnchors(ctx, db, []AnchorDeclaration{decl}, versionID, chunks, doc)
	if err != nil {
		t.Fatalf("failed to create second anchor: %v", err)
	}

	// Should still have only 1 anchor (deduplication worked)
	records, err := ActiveAnchors(ctx, db, "workspace")
	if err != nil {
		t.Fatalf("failed to query anchors: %v", err)
	}

	if len(records) != 1 {
		t.Errorf("expected 1 anchor (deduplication), got %d", len(records))
	}
}

func Test_AnchorHistory(t *testing.T) {
	db := setupAnchorTestDB(t)
	defer db.Close()

	ctx := context.Background()

	doc := DocumentRecord{
		DocID:       "test-doc-1",
		CorpusScope: "workspace",
	}
	chunks := []ChunkRecord{}
	versionID := "v1"

	// Create initial anchor
	decl1 := AnchorDeclaration{
		Term:       "verified",
		Definition: "definition 1",
		Class:      "policy",
	}

	err := extractAndPersistAnchors(ctx, db, []AnchorDeclaration{decl1}, versionID, chunks, doc)
	if err != nil {
		t.Fatalf("failed to create anchor: %v", err)
	}

	// Get the anchor and supersede it multiple times
	anchors, err := ActiveAnchors(ctx, db, "workspace")
	if err != nil {
		t.Fatalf("failed to query anchors: %v", err)
	}

	currentID := anchors[0].AnchorID

	// Create 2 more supersessions
	_, err = SupersedeAnchor(ctx, db, currentID, "definition 2", nil)
	if err != nil {
		t.Fatalf("failed to supersede anchor: %v", err)
	}

	anchors, err = ActiveAnchors(ctx, db, "workspace")
	if err != nil {
		t.Fatalf("failed to query anchors: %v", err)
	}
	currentID = anchors[0].AnchorID

	_, err = SupersedeAnchor(ctx, db, currentID, "definition 3", nil)
	if err != nil {
		t.Fatalf("failed to supersede anchor: %v", err)
	}

	// Get full history
	history, err := AnchorHistory(ctx, db, "verified", "workspace")
	if err != nil {
		t.Fatalf("failed to get anchor history: %v", err)
	}

	if len(history) < 3 {
		t.Errorf("expected at least 3 anchors in history, got %d", len(history))
	}
}

func Test_AnchorInvalidation(t *testing.T) {
	db := setupAnchorTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create an anchor
	decl := AnchorDeclaration{
		Term:       "verified",
		Definition: "definition",
		Class:      "policy",
	}

	doc := DocumentRecord{
		DocID:       "test-doc-1",
		CorpusScope: "workspace",
	}
	chunks := []ChunkRecord{}
	versionID := "v1"

	err := extractAndPersistAnchors(ctx, db, []AnchorDeclaration{decl}, versionID, chunks, doc)
	if err != nil {
		t.Fatalf("failed to create anchor: %v", err)
	}

	// Get the anchor
	records, err := ActiveAnchors(ctx, db, "workspace")
	if err != nil {
		t.Fatalf("failed to query anchors: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 anchor, got %d", len(records))
	}

	anchorID := records[0].AnchorID

	// Invalidate the anchor
	err = InvalidateAnchor(ctx, db, anchorID, "no longer relevant")
	if err != nil {
		t.Fatalf("failed to invalidate anchor: %v", err)
	}

	// Should no longer be in active anchors
	records, err = ActiveAnchors(ctx, db, "workspace")
	if err != nil {
		t.Fatalf("failed to query anchors: %v", err)
	}

	if len(records) != 0 {
		t.Errorf("expected 0 active anchors after invalidation, got %d", len(records))
	}
}

func Test_AnchorsForTerms(t *testing.T) {
	db := setupAnchorTestDB(t)
	defer db.Close()

	ctx := context.Background()

	doc := DocumentRecord{
		DocID:       "test-doc-1",
		CorpusScope: "workspace",
	}
	chunks := []ChunkRecord{}
	versionID := "v1"

	// Create multiple anchors
	decls := []AnchorDeclaration{
		{
			Term:       "verified",
			Definition: "def1",
			Class:      "policy",
		},
		{
			Term:       "approved",
			Definition: "def2",
			Class:      "policy",
		},
		{
			Term:       "owner",
			Definition: "def3",
			Class:      "identity",
		},
	}

	err := extractAndPersistAnchors(ctx, db, decls, versionID, chunks, doc)
	if err != nil {
		t.Fatalf("failed to create anchors: %v", err)
	}

	// Query for specific terms
	refs, err := AnchorsForTerms(ctx, db, []string{"verified", "owner"}, "workspace")
	if err != nil {
		t.Fatalf("failed to query for terms: %v", err)
	}

	if len(refs) != 2 {
		t.Errorf("expected 2 refs, got %d", len(refs))
	}

	// Check that we got the right terms
	termMap := make(map[string]bool)
	for _, ref := range refs {
		termMap[ref.Term] = true
	}

	if !termMap["verified"] {
		t.Errorf("expected 'verified' in results")
	}
	if !termMap["owner"] {
		t.Errorf("expected 'owner' in results")
	}
	if termMap["approved"] {
		t.Errorf("did not expect 'approved' in results")
	}
}

func Test_SchemaMigrationV5(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Ensure schema (which should create V5 tables)
	err = EnsureSchema(ctx, db)
	if err != nil {
		t.Fatalf("failed to ensure schema: %v", err)
	}

	// Verify schema version
	version, err := CurrentSchemaVersion(ctx, db)
	if err != nil {
		t.Fatalf("failed to get schema version: %v", err)
	}

	if version != 5 {
		t.Errorf("expected schema version 5, got %d", version)
	}

	// Verify anchor tables exist
	tables := []string{"retrieval_semantic_anchors", "retrieval_anchor_events"}
	for _, table := range tables {
		exists, err := tableExists(ctx, db, table)
		if err != nil {
			t.Fatalf("failed to check table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("expected table %s to exist", table)
		}
	}
}
