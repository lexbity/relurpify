// Package thoughtrecipe implements the Euclo thoughtrecipe DSL.
package thoughtrecipe

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// SourceBuffer holds the immutable original source bytes of a recipe file.
// It preserves all bytes exactly, including line endings (CRLF vs LF), comments,
// and whitespace trivia. This is the canonical representation used for
// lossless editing and roundtrip fidelity (P04/D-10).
type SourceBuffer struct {
	// bytes is the complete, unmodified source file content.
	bytes []byte
	// filename is the path identifier for this source.
	filename string
	// digest is the SHA-256 hash of bytes, computed once and cached.
	digest string
}

// NewSourceBuffer creates a new immutable source buffer from raw bytes.
// The bytes are NOT copied; the caller must not mutate the passed slice.
func NewSourceBuffer(filename string, bytes []byte) *SourceBuffer {
	if bytes == nil {
		bytes = []byte{}
	}
	digest := sha256.Sum256(bytes)
	return &SourceBuffer{
		bytes:    bytes,
		filename: filename,
		digest:   hex.EncodeToString(digest[:]),
	}
}

// NewSourceBufferFromString creates a new source buffer from a string.
// The string is converted to bytes and stored as-is.
func NewSourceBufferFromString(filename, content string) *SourceBuffer {
	return NewSourceBuffer(filename, []byte(content))
}

// NewSourceBufferFromFile reads a file and creates a source buffer with exact bytes.
func NewSourceBufferFromFile(path string) (*SourceBuffer, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", path, err)
	}
	return NewSourceBuffer(path, content), nil
}

// Bytes returns the immutable source bytes.
func (s *SourceBuffer) Bytes() []byte {
	return s.bytes
}

// String returns the source as a string.
func (s *SourceBuffer) String() string {
	return string(s.bytes)
}

// Filename returns the source file path.
func (s *SourceBuffer) Filename() string {
	return s.filename
}

// Digest returns the SHA-256 hex digest of the source bytes.
// This uniquely identifies the exact byte content for revision tracking.
func (s *SourceBuffer) Digest() string {
	return s.digest
}

// Len returns the number of bytes in the source.
func (s *SourceBuffer) Len() int {
	return len(s.bytes)
}

// Slice returns a slice of the source bytes from start to end (half-open interval).
// Panics if indices are out of bounds.
func (s *SourceBuffer) Slice(start, end int) []byte {
	return s.bytes[start:end]
}

// ByteAt returns the byte at position i.
// Panics if i is out of bounds.
func (s *SourceBuffer) ByteAt(i int) byte {
	return s.bytes[i]
}

// Equal reports whether s and other have identical content.
func (s *SourceBuffer) Equal(other *SourceBuffer) bool {
	if s == nil || other == nil {
		return s == other
	}
	if s.digest == other.digest {
		return true
	}
	return string(s.bytes) == string(other.bytes)
}

// ByteSpan represents a byte range in source text for lossless editing.
// Start is inclusive, End is exclusive. This is distinct from the AST SourceSpan
// which uses SourceLocation (line/column). This byte-based span is used for
// all editing operations to ensure exact roundtrip fidelity (P04/D-10).
type ByteSpan struct {
	Start int
	End   int
}

// NewByteSpan creates a new byte span.
// Does not panic; use IsValid() to check if the span is valid.
func NewByteSpan(start, end int) ByteSpan {
	return ByteSpan{Start: start, End: end}
}

// Len returns the length of the span (end - start).
func (s ByteSpan) Len() int {
	if s.Start > s.End {
		return 0
	}
	return s.End - s.Start
}

// IsValid reports whether the span is valid (start <= end).
func (s ByteSpan) IsValid() bool {
	return s.Start <= s.End
}

// Contains reports whether the span contains the given byte offset.
// The offset is contained if start <= offset < end.
func (s ByteSpan) Contains(offset int) bool {
	return s.Start <= offset && offset < s.End
}

// Overlaps reports whether the span overlaps with other.
func (s ByteSpan) Overlaps(other ByteSpan) bool {
	return s.Start < other.End && other.Start < s.End
}

// DocumentSource holds a source buffer paired with its parsed Document AST.
// This combines the canonical byte representation (for lossless editing) with
// the parsed structure (for semantic operations). The Digest is precomputed
// from the buffer for efficient identity checks (P04/D-10).
type DocumentSource struct {
	// Buffer is the immutable canonical source bytes.
	Buffer *SourceBuffer
	// Document is the parsed AST from the source.
	Document *Document
	// Digest is the SHA-256 hex digest of Buffer.Bytes().
	Digest string
}

// NewDocumentSource creates a DocumentSource from a buffer and parsed document.
// The digest is taken from the buffer.
func NewDocumentSource(buffer *SourceBuffer, doc *Document) *DocumentSource {
	return &DocumentSource{
		Buffer:   buffer,
		Document: doc,
		Digest:   buffer.Digest(),
	}
}

// ParseFile reads and parses a recipe file, preserving exact bytes.
func ParseFile(path string) (*DocumentSource, error) {
	buffer, err := NewSourceBufferFromFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := ParseSource(buffer.Filename(), buffer.String())
	if err != nil {
		return nil, err
	}
	return NewDocumentSource(buffer, doc), nil
}

// ParseSourceWithBuffer parses a source string and returns a DocumentSource.
// This preserves the exact bytes for lossless editing.
func ParseSourceWithBuffer(filename, src string) (*DocumentSource, error) {
	buffer := NewSourceBufferFromString(filename, src)
	doc, err := ParseSource(filename, src)
	if err != nil {
		return nil, err
	}
	return NewDocumentSource(buffer, doc), nil
}
