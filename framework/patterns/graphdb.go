package patterns

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/framework/graphdb"
)

const (
	nodeKindPattern     graphdb.NodeKind = "pattern_record"
	nodeKindComment     graphdb.NodeKind = "pattern_comment"
	edgeKindSupersedes  graphdb.EdgeKind = "supersedes"
	edgeKindHasComment  graphdb.EdgeKind = "has_comment"
)

// GraphDBPatternStore implements PatternStore using graphdb.Engine.
type GraphDBPatternStore struct {
	Graph *graphdb.Engine
}

// NewGraphDBPatternStore creates a new graphdb-backed pattern store.
func NewGraphDBPatternStore(engine *graphdb.Engine) *GraphDBPatternStore {
	return &GraphDBPatternStore{Graph: engine}
}

func (s *GraphDBPatternStore) Save(ctx context.Context, record PatternRecord) error {
	if s.Graph == nil {
		return fmt.Errorf("graphdb pattern store: graph engine is required")
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now

	// Set status label
	labels := []string{fmt.Sprintf("status:%s", record.Status), fmt.Sprintf("kind:%s", record.Kind)}

	props, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal pattern record: %w", err)
	}

	return s.Graph.UpsertNode(graphdb.NodeRecord{
		ID:       record.ID,
		Kind:     nodeKindPattern,
		Labels:   labels,
		Props:    props,
		SourceID: record.CorpusScope,
	})
}

func (s *GraphDBPatternStore) Load(ctx context.Context, id string) (*PatternRecord, error) {
	if s.Graph == nil {
		return nil, fmt.Errorf("graphdb pattern store: graph engine is required")
	}
	node, ok := s.Graph.GetNode(id)
	if !ok {
		return nil, nil
	}

	var record PatternRecord
	if err := json.Unmarshal(node.Props, &record); err != nil {
		return nil, fmt.Errorf("unmarshal pattern record: %w", err)
	}
	return &record, nil
}

func (s *GraphDBPatternStore) ListByStatus(ctx context.Context, status PatternStatus, corpusScope string) ([]PatternRecord, error) {
	if s.Graph == nil {
		return nil, nil
	}
	nodes := s.Graph.ListNodes(nodeKindPattern)
	var out []PatternRecord
	for _, node := range nodes {
		// Check corpus scope
		if corpusScope != "" && node.SourceID != corpusScope {
			continue
		}
		// Check status label
		statusLabel := fmt.Sprintf("status:%s", status)
		hasStatus := false
		for _, label := range node.Labels {
			if label == statusLabel {
				hasStatus = true
				break
			}
		}
		if !hasStatus {
			continue
		}

		var record PatternRecord
		if err := json.Unmarshal(node.Props, &record); err != nil {
			continue
		}
		out = append(out, record)
	}
	return out, nil
}

func (s *GraphDBPatternStore) ListByKind(ctx context.Context, kind PatternKind, corpusScope string) ([]PatternRecord, error) {
	if s.Graph == nil {
		return nil, nil
	}
	nodes := s.Graph.ListNodes(nodeKindPattern)
	var out []PatternRecord
	for _, node := range nodes {
		// Check corpus scope
		if corpusScope != "" && node.SourceID != corpusScope {
			continue
		}
		// Check kind label
		kindLabel := fmt.Sprintf("kind:%s", kind)
		hasKind := false
		for _, label := range node.Labels {
			if label == kindLabel {
				hasKind = true
				break
			}
		}
		if !hasKind {
			continue
		}

		var record PatternRecord
		if err := json.Unmarshal(node.Props, &record); err != nil {
			continue
		}
		out = append(out, record)
	}
	return out, nil
}

func (s *GraphDBPatternStore) UpdateStatus(ctx context.Context, id string, status PatternStatus, confirmedBy string) error {
	if s.Graph == nil {
		return fmt.Errorf("graphdb pattern store: graph engine is required")
	}
	record, err := s.Load(ctx, id)
	if err != nil {
		return err
	}
	if record == nil {
		return fmt.Errorf("pattern record %q not found", id)
	}

	record.Status = status
	record.UpdatedAt = time.Now().UTC()
	if status == PatternStatusConfirmed && confirmedBy != "" {
		record.ConfirmedBy = confirmedBy
		now := time.Now().UTC()
		record.ConfirmedAt = &now
	}

	return s.Save(ctx, *record)
}

func (s *GraphDBPatternStore) Supersede(ctx context.Context, oldID string, replacement PatternRecord) error {
	if s.Graph == nil {
		return fmt.Errorf("graphdb pattern store: graph engine is required")
	}
	// Load old record and mark as superseded
	oldRecord, err := s.Load(ctx, oldID)
	if err != nil {
		return err
	}
	if oldRecord == nil {
		return fmt.Errorf("pattern record %q not found", oldID)
	}

	// Save the replacement first
	replacement.ID = replacement.ID // ensure it has its own ID
	if err := s.Save(ctx, replacement); err != nil {
		return fmt.Errorf("save replacement pattern: %w", err)
	}

	// Update old record status
	oldRecord.Status = PatternStatusSuperseded
	oldRecord.SupersededBy = replacement.ID
	oldRecord.UpdatedAt = time.Now().UTC()
	if err := s.Save(ctx, *oldRecord); err != nil {
		return fmt.Errorf("update superseded pattern: %w", err)
	}

	// Create supersession edge
	edge := graphdb.EdgeRecord{
		SourceID: oldID,
		TargetID: replacement.ID,
		Kind:     edgeKindSupersedes,
		CreatedAt: time.Now().UTC().UnixNano(),
	}
	return s.Graph.LinkEdges([]graphdb.EdgeRecord{edge})
}

// GraphDBCommentStore implements CommentStore using graphdb.Engine.
type GraphDBCommentStore struct {
	Graph *graphdb.Engine
}

// NewGraphDBCommentStore creates a new graphdb-backed comment store.
func NewGraphDBCommentStore(engine *graphdb.Engine) *GraphDBCommentStore {
	return &GraphDBCommentStore{Graph: engine}
}

func (s *GraphDBCommentStore) Save(ctx context.Context, record CommentRecord) error {
	if s.Graph == nil {
		return fmt.Errorf("graphdb comment store: graph engine is required")
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now

	props, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal comment record: %w", err)
	}

	return s.Graph.UpsertNode(graphdb.NodeRecord{
		ID:       record.CommentID,
		Kind:     nodeKindComment,
		Labels:   []string{fmt.Sprintf("intent:%s", record.IntentType)},
		Props:    props,
		SourceID: record.CorpusScope,
	})
}

func (s *GraphDBCommentStore) Load(ctx context.Context, id string) (*CommentRecord, error) {
	if s.Graph == nil {
		return nil, fmt.Errorf("graphdb comment store: graph engine is required")
	}
	node, ok := s.Graph.GetNode(id)
	if !ok {
		return nil, nil
	}

	var record CommentRecord
	if err := json.Unmarshal(node.Props, &record); err != nil {
		return nil, fmt.Errorf("unmarshal comment record: %w", err)
	}
	return &record, nil
}

func (s *GraphDBCommentStore) ListForPattern(ctx context.Context, patternID string) ([]CommentRecord, error) {
	// For now, scan all comments and filter by patternID
	// In a full implementation, we'd use edges from pattern to comments
	return s.listByFilter(func(record CommentRecord) bool {
		return record.PatternID == patternID
	})
}

func (s *GraphDBCommentStore) ListForAnchor(ctx context.Context, anchorID string) ([]CommentRecord, error) {
	return s.listByFilter(func(record CommentRecord) bool {
		return record.AnchorID == anchorID
	})
}

func (s *GraphDBCommentStore) ListForTension(ctx context.Context, tensionID string) ([]CommentRecord, error) {
	return s.listByFilter(func(record CommentRecord) bool {
		return record.TensionID == tensionID
	})
}

func (s *GraphDBCommentStore) ListForSymbol(ctx context.Context, symbolID string) ([]CommentRecord, error) {
	return s.listByFilter(func(record CommentRecord) bool {
		return record.SymbolID == symbolID
	})
}

func (s *GraphDBCommentStore) listByFilter(filter func(CommentRecord) bool) ([]CommentRecord, error) {
	if s.Graph == nil {
		return nil, nil
	}
	nodes := s.Graph.ListNodes(nodeKindComment)
	var out []CommentRecord
	for _, node := range nodes {
		var record CommentRecord
		if err := json.Unmarshal(node.Props, &record); err != nil {
			continue
		}
		if filter(record) {
			out = append(out, record)
		}
	}
	return out, nil
}
