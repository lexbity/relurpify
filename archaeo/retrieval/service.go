package archaeoretrieval

import (
	"context"
	"database/sql"

	frameworkretrieval "codeburg.org/lexbit/relurpify/framework/retrieval"
)

type Store interface {
	ActiveAnchors(context.Context, string) ([]frameworkretrieval.AnchorRecord, error)
	DriftedAnchors(context.Context, string) ([]frameworkretrieval.AnchorRecord, error)
	UnresolvedDrifts(context.Context, string) ([]frameworkretrieval.AnchorEventRecord, error)
	DeclareAnchor(context.Context, frameworkretrieval.AnchorDeclaration, string, string) (*frameworkretrieval.AnchorRecord, error)
	InvalidateAnchor(context.Context, string, string) error
	SupersedeAnchor(context.Context, string, string, map[string]string) (*frameworkretrieval.AnchorRecord, error)
}

type SQLStore struct {
	db *sql.DB
}

func NewSQLStore(db *sql.DB) *SQLStore {
	if db == nil {
		return nil
	}
	return &SQLStore{db: db}
}

func SQLDB(store Store) *sql.DB {
	if typed, ok := store.(*SQLStore); ok && typed != nil {
		return typed.db
	}
	return nil
}

func (s *SQLStore) ActiveAnchors(ctx context.Context, corpusScope string) ([]frameworkretrieval.AnchorRecord, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	return frameworkretrieval.ActiveAnchors(ctx, s.db, corpusScope)
}

func (s *SQLStore) DriftedAnchors(ctx context.Context, corpusScope string) ([]frameworkretrieval.AnchorRecord, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	return frameworkretrieval.DriftedAnchors(ctx, s.db, corpusScope)
}

func (s *SQLStore) UnresolvedDrifts(ctx context.Context, corpusScope string) ([]frameworkretrieval.AnchorEventRecord, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	return frameworkretrieval.UnresolvedDrifts(ctx, s.db, corpusScope)
}

func (s *SQLStore) DeclareAnchor(ctx context.Context, decl frameworkretrieval.AnchorDeclaration, corpusScope, trustClass string) (*frameworkretrieval.AnchorRecord, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	return frameworkretrieval.DeclareAnchor(ctx, s.db, decl, corpusScope, trustClass)
}

func (s *SQLStore) InvalidateAnchor(ctx context.Context, anchorID string, reason string) error {
	if s == nil || s.db == nil {
		return nil
	}
	return frameworkretrieval.InvalidateAnchor(ctx, s.db, anchorID, reason)
}

func (s *SQLStore) SupersedeAnchor(ctx context.Context, anchorID string, newDefinition string, newContext map[string]string) (*frameworkretrieval.AnchorRecord, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	return frameworkretrieval.SupersedeAnchor(ctx, s.db, anchorID, newDefinition, newContext)
}
