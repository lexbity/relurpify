package runtime

// types.go defines shared types for the runtime package.

// Cursor represents a monotonic position in the journal.
type Cursor uint64

// RequestID is a 128-bit identifier with req_ prefix per D-08.
type RequestID string

// WorkspaceID identifies a workspace.
type WorkspaceID string
