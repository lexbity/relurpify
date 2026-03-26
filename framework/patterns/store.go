package patterns

import "context"

type PatternStore interface {
	Save(ctx context.Context, record PatternRecord) error
	Load(ctx context.Context, id string) (*PatternRecord, error)
	ListByStatus(ctx context.Context, status PatternStatus, corpusScope string) ([]PatternRecord, error)
	ListByKind(ctx context.Context, kind PatternKind, corpusScope string) ([]PatternRecord, error)
	UpdateStatus(ctx context.Context, id string, status PatternStatus, confirmedBy string) error
	Supersede(ctx context.Context, oldID string, replacement PatternRecord) error
}

type CommentStore interface {
	Save(ctx context.Context, record CommentRecord) error
	Load(ctx context.Context, id string) (*CommentRecord, error)
	ListForPattern(ctx context.Context, patternID string) ([]CommentRecord, error)
	ListForAnchor(ctx context.Context, anchorID string) ([]CommentRecord, error)
	ListForSymbol(ctx context.Context, symbolID string) ([]CommentRecord, error)
}
