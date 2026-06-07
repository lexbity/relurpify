package retrieval

// AnchorRef represents a semantic anchor used to scope retrieval queries.
type AnchorRef struct {
	AnchorID   string `json:"anchor_id"`
	ChunkID    string `json:"chunk_id,omitempty"`
	Term       string `json:"term"`
	Definition string `json:"definition"`
	Class      string `json:"class"`
	Active     bool   `json:"active"`
	CreatedAt  string `json:"created_at"`
}
