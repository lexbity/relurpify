package patterns

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func openTestStores(t *testing.T) (*SQLitePatternStore, *SQLiteCommentStore) {
	t.Helper()
	db, err := OpenSQLite(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	patterns, err := NewSQLitePatternStore(db)
	require.NoError(t, err)
	comments, err := NewSQLiteCommentStore(db)
	require.NoError(t, err)
	return patterns, comments
}

func TestSQLitePatternStoreRoundTripAndQueries(t *testing.T) {
	patterns, _ := openTestStores(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Nanosecond)
	record := PatternRecord{
		ID:          "pattern-1",
		Kind:        PatternKindStructural,
		Title:       "ErrorWrap",
		Description: "wrap errors at boundaries",
		Status:      PatternStatusProposed,
		Instances: []PatternInstance{{
			FilePath: "main.go", StartLine: 10, EndLine: 12, Excerpt: "return fmt.Errorf(...)",
		}},
		CommentIDs:   []string{"comment-1"},
		AnchorRefs:   []string{"anchor:error-wrap"},
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		Confidence:   0.82,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, patterns.Save(ctx, record))

	loaded, err := patterns.Load(ctx, record.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, record.Title, loaded.Title)
	require.Equal(t, record.Instances, loaded.Instances)
	require.Equal(t, record.CommentIDs, loaded.CommentIDs)

	proposed, err := patterns.ListByStatus(ctx, PatternStatusProposed, "workspace")
	require.NoError(t, err)
	require.Len(t, proposed, 1)

	structural, err := patterns.ListByKind(ctx, PatternKindStructural, "workspace")
	require.NoError(t, err)
	require.Len(t, structural, 1)

	require.NoError(t, patterns.UpdateStatus(ctx, record.ID, PatternStatusConfirmed, "human"))
	loaded, err = patterns.Load(ctx, record.ID)
	require.NoError(t, err)
	require.Equal(t, PatternStatusConfirmed, loaded.Status)
	require.Equal(t, "human", loaded.ConfirmedBy)
	require.NotNil(t, loaded.ConfirmedAt)

	replacement := PatternRecord{
		ID:           "pattern-2",
		Kind:         PatternKindStructural,
		Title:        "ErrorWrap v2",
		Description:  "better wrapping",
		Status:       PatternStatusConfirmed,
		CorpusScope:  "workspace",
		CorpusSource: "workspace",
		CreatedAt:    now.Add(time.Second),
		UpdatedAt:    now.Add(time.Second),
	}
	require.NoError(t, patterns.Supersede(ctx, "pattern-1", replacement))

	oldRecord, err := patterns.Load(ctx, "pattern-1")
	require.NoError(t, err)
	require.Equal(t, PatternStatusSuperseded, oldRecord.Status)
	require.Equal(t, "pattern-2", oldRecord.SupersededBy)

	newRecord, err := patterns.Load(ctx, "pattern-2")
	require.NoError(t, err)
	require.Equal(t, "ErrorWrap v2", newRecord.Title)
}

func TestSQLiteCommentStoreRoundTripAndQueries(t *testing.T) {
	_, comments := openTestStores(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Nanosecond)

	first := CommentRecord{
		CommentID:   "comment-1",
		PatternID:   "pattern-1",
		AnchorID:    "anchor-1",
		FilePath:    "main.go",
		SymbolID:    "pkg.Func",
		IntentType:  CommentIntentional,
		Body:        "ErrorWrap: boundary rule",
		AuthorKind:  AuthorKindHuman,
		TrustClass:  TrustClassWorkspaceTrusted,
		AnchorRef:   "anchor:error-wrap",
		CorpusScope: "workspace",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	second := CommentRecord{
		CommentID:  "comment-2",
		PatternID:  "pattern-2",
		AnchorID:   "anchor-2",
		TensionID:  "tension-2",
		SymbolID:   "pkg.Other",
		IntentType: CommentDeferred,
		Body:       "later",
		AuthorKind: AuthorKindAgent,
		TrustClass: TrustClassBuiltinTrusted,
		CreatedAt:  now.Add(time.Second),
		UpdatedAt:  now.Add(time.Second),
	}
	require.NoError(t, comments.Save(ctx, first))
	require.NoError(t, comments.Save(ctx, second))

	loaded, err := comments.Load(ctx, "comment-1")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, first.Body, loaded.Body)
	require.Equal(t, first.TrustClass, loaded.TrustClass)

	forPattern, err := comments.ListForPattern(ctx, "pattern-1")
	require.NoError(t, err)
	require.Len(t, forPattern, 1)
	require.Equal(t, "comment-1", forPattern[0].CommentID)

	forAnchor, err := comments.ListForAnchor(ctx, "anchor-1")
	require.NoError(t, err)
	require.Len(t, forAnchor, 1)
	require.Equal(t, "comment-1", forAnchor[0].CommentID)

	forTension, err := comments.ListForTension(ctx, "tension-2")
	require.NoError(t, err)
	require.Len(t, forTension, 1)
	require.Equal(t, "comment-2", forTension[0].CommentID)

	forSymbol, err := comments.ListForSymbol(ctx, "pkg.Func")
	require.NoError(t, err)
	require.Len(t, forSymbol, 1)
	require.Equal(t, "comment-1", forSymbol[0].CommentID)
}
