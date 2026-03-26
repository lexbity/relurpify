package graphdb

import "encoding/json"

// NodeKind and EdgeKind are opaque typed strings. The engine assigns no meaning
// to their values.
type NodeKind string
type EdgeKind string

// Direction controls traversal direction in queries.
type Direction string

const (
	DirectionOut  Direction = "out"
	DirectionIn   Direction = "in"
	DirectionBoth Direction = "both"
)

type NodeRecord struct {
	ID        string          `json:"id"`
	Kind      NodeKind        `json:"kind"`
	SourceID  string          `json:"source_id,omitempty"`
	Labels    []string        `json:"labels,omitempty"`
	Props     json.RawMessage `json:"props,omitempty"`
	CreatedAt int64           `json:"created_at"`
	UpdatedAt int64           `json:"updated_at"`
	DeletedAt int64           `json:"deleted_at,omitempty"`
}

type EdgeRecord struct {
	SourceID  string          `json:"s"`
	TargetID  string          `json:"t"`
	Kind      EdgeKind        `json:"k"`
	Weight    float32         `json:"w,omitempty"`
	Props     json.RawMessage `json:"p,omitempty"`
	CreatedAt int64           `json:"c"`
	DeletedAt int64           `json:"d,omitempty"`
}

func (e EdgeRecord) IsActive() bool {
	return e.DeletedAt == 0
}

type PathResult struct {
	Source string       `json:"source"`
	Target string       `json:"target"`
	Path   []string     `json:"path"`
	Edges  []EdgeRecord `json:"edges"`
}

type GraphQuery struct {
	RootIDs   []string   `json:"root_ids"`
	EdgeKinds []EdgeKind `json:"edge_kinds"`
	Direction Direction  `json:"direction"`
	MaxDepth  int        `json:"max_depth"`
}

type ImpactResult struct {
	OriginIDs []string         `json:"origin_ids"`
	Affected  []string         `json:"affected"`
	ByDepth   map[int][]string `json:"by_depth"`
}
