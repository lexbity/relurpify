package ingestion

// IngestionMode controls the scope of file ingestion.
type IngestionMode string

const (
	// IngestionModeFilesOnly ingests only explicitly selected user files.
	IngestionModeFilesOnly IngestionMode = "files_only"
	// IngestionModeIncremental scans workspace incrementally (git-diff since last run).
	IngestionModeIncremental IngestionMode = "incremental"
	// IngestionModeFull performs a full workspace scan (expensive).
	IngestionModeFull IngestionMode = "full"
)

// FileIngestionRecord tracks a single file that was ingested.
type FileIngestionRecord struct {
	Path         string // File path
	ChunkCount   int    // Number of chunks generated
	SizeBytes    int64  // File size in bytes
	IngestedAt   int64  // Unix timestamp
	ContentHash  string // Hash of file content for deduplication
}

// IngestionResult is the result of an ingestion operation.
type IngestionResult struct {
	Mode        IngestionMode          // The mode used for ingestion
	FileCount   int                    // Total files ingested
	ChunkCount  int                    // Total chunks generated
	Records     []FileIngestionRecord  // Individual file records
	SinceRef    string                 // Git ref used for incremental mode (if applicable)
	Error       string                 // Error message if ingestion failed
	CompletedAt int64                  // Unix timestamp of completion
}
