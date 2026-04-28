package ast

import "time"

// FileMetadata stores per-file statistics and indexing metadata.
// This is the unified struct definition that combines fields from both
// the graphdb-backed indexing model (file_metadata.go) and the CodeIndex
// interface contract (index_types.go).
type FileMetadata struct {
	ID            string    `json:"id"`
	Path          string    `json:"path"`
	RelativePath  string    `json:"relative_path"`
	Language      string    `json:"language"`
	Category      Category  `json:"category"`
	LineCount     int       `json:"line_count"`
	LOC           int       `json:"loc"` // Lines of code from CodeIndex contract
	TokenCount    int       `json:"token_count"`
	Complexity    int       `json:"complexity"`
	ContentHash   string    `json:"content_hash"`
	Hash          string    `json:"hash"` // Legacy hash field from CodeIndex contract
	RootNodeID    string    `json:"root_node_id"`
	NodeCount     int       `json:"node_count"`
	EdgeCount     int       `json:"edge_count"`
	IndexedAt     time.Time `json:"indexed_at"`
	LastIndexed   time.Time `json:"last_indexed"`  // Legacy field from CodeIndex contract
	LastModified  time.Time `json:"last_modified"` // File modification time
	Size          int64     `json:"size"`          // File size in bytes
	ParserVersion string    `json:"parser_version"`
	Summary       string    `json:"summary"`
	SummaryHash   string    `json:"summary_hash"`
	Symbols       []Symbol  `json:"symbols,omitempty"` // Symbols from CodeIndex contract
	Imports       []string  `json:"imports,omitempty"` // Import statements
}
