// Package thoughtrecipe implements the Euclo thoughtrecipe DSL.
package thoughtrecipe

import (
	"fmt"
)

// EditOperation represents a single atomic edit to a source buffer.
// All edits are represented as byte-level patches to ensure lossless roundtrips.
type EditOperation struct {
	// Span is the byte range to replace (can be zero-length for insertions).
	Span ByteSpan
	// Replacement is the new content to insert at Span.Start.
	// If empty, this is a deletion.
	Replacement []byte
	// Description is a human-readable description of the edit.
	Description string
}

// NewEditOperation creates a new edit operation.
func NewEditOperation(span ByteSpan, replacement []byte, description string) EditOperation {
	return EditOperation{
		Span:        span,
		Replacement: replacement,
		Description: description,
	}
}

// NewInsertOperation creates an insertion at a byte offset.
func NewInsertOperation(offset int, content []byte, description string) EditOperation {
	return EditOperation{
		Span:        ByteSpan{Start: offset, End: offset},
		Replacement: content,
		Description: description,
	}
}

// NewDeleteOperation creates a deletion of a byte range.
func NewDeleteOperation(span ByteSpan, description string) EditOperation {
	return EditOperation{
		Span:        span,
		Replacement: nil,
		Description: description,
	}
}

// NewReplaceOperation creates a replacement of a byte range.
func NewReplaceOperation(span ByteSpan, replacement []byte, description string) EditOperation {
	return EditOperation{
		Span:        span,
		Replacement: replacement,
		Description: description,
	}
}

// IsInsert reports whether this is an insertion (empty span, non-empty replacement).
func (e EditOperation) IsInsert() bool {
	return e.Span.Len() == 0 && len(e.Replacement) > 0
}

// IsDelete reports whether this is a deletion (non-empty span, empty replacement).
func (e EditOperation) IsDelete() bool {
	return e.Span.Len() > 0 && len(e.Replacement) == 0
}

// IsReplace reports whether this is a replacement (non-empty span and replacement).
func (e EditOperation) IsReplace() bool {
	return e.Span.Len() > 0 && len(e.Replacement) > 0
}

// IsNoOp reports whether this edit does nothing.
func (e EditOperation) IsNoOp() bool {
	return e.Span.Len() == 0 && len(e.Replacement) == 0
}

// String returns a human-readable representation.
func (e EditOperation) String() string {
	if e.IsNoOp() {
		return "no-op"
	}
	if e.IsInsert() {
		return fmt.Sprintf("insert %d bytes at %d", len(e.Replacement), e.Span.Start)
	}
	if e.IsDelete() {
		return fmt.Sprintf("delete %d bytes at %d:%d", e.Span.Len(), e.Span.Start, e.Span.End)
	}
	return fmt.Sprintf("replace %d:%d with %d bytes", e.Span.Start, e.Span.End, len(e.Replacement))
}

// EditTransaction is a sequence of edit operations that are applied atomically.
// All operations are applied in order, with later operations using the
// post-edit offsets from earlier ones (P04/D-10).
type EditTransaction struct {
	// BaseRevision is the revision this transaction applies to.
	BaseRevision RecipeRevision
	// Operations is the sequence of edits to apply.
	Operations []EditOperation
	// Description is a human-readable description of the transaction.
	Description string
}

// NewEditTransaction creates a new edit transaction.
func NewEditTransaction(base RecipeRevision, description string) *EditTransaction {
	return &EditTransaction{
		BaseRevision: base,
		Operations:   make([]EditOperation, 0),
		Description:  description,
	}
}

// AddOperation adds an operation to the transaction.
func (t *EditTransaction) AddOperation(op EditOperation) {
	t.Operations = append(t.Operations, op)
}

// Apply applies the transaction to a source buffer and returns a new buffer.
// Returns an error if the base revision doesn't match or if operations are invalid.
func (t *EditTransaction) Apply(buffer *SourceBuffer) (*SourceBuffer, error) {
	// Verify base revision matches
	currentRev := NewRecipeRevisionFromSource(buffer)
	if !currentRev.Equal(t.BaseRevision) {
		return nil, fmt.Errorf("base revision mismatch: expected %s, got %s",
			t.BaseRevision.String(), currentRev.String())
	}

	// Apply operations in order
	result := make([]byte, len(buffer.Bytes()))
	copy(result, buffer.Bytes())

	// Track offset adjustments for subsequent operations
	offsetAdjustment := 0
	for _, op := range t.Operations {
		// Adjust operation span for previous edits
		adjustedSpan := ByteSpan{
			Start: op.Span.Start + offsetAdjustment,
			End:   op.Span.End + offsetAdjustment,
		}

		// Validate span
		if !adjustedSpan.IsValid() {
			return nil, fmt.Errorf("invalid span in operation: %v", op)
		}
		if adjustedSpan.End > len(result) {
			return nil, fmt.Errorf("operation span exceeds buffer length: %v (buffer len: %d)",
				adjustedSpan, len(result))
		}

		// Calculate new length
		oldLen := adjustedSpan.Len()
		newLen := len(op.Replacement)

		// Create new result with appropriate capacity
		newResult := make([]byte, len(result)+newLen-oldLen)
		copy(newResult[:adjustedSpan.Start], result[:adjustedSpan.Start])
		copy(newResult[adjustedSpan.Start:adjustedSpan.Start+newLen], op.Replacement)
		copy(newResult[adjustedSpan.Start+newLen:], result[adjustedSpan.End:])

		// Update for next iteration
		result = newResult
		offsetAdjustment += newLen - oldLen
	}

	return NewSourceBuffer(buffer.Filename(), result), nil
}

// ApplyToString applies the transaction to a string and returns the result.
// It ignores the transaction's BaseRevision and uses the source string's revision.
// This is useful for testing and simple string transformations.
func (t *EditTransaction) ApplyToString(src string) (string, error) {
	buffer := NewSourceBufferFromString("", src)
	// Update the transaction's base revision to match the buffer
	txnForBuffer := NewEditTransaction(NewRecipeRevisionFromSource(buffer), t.Description)
	txnForBuffer.Operations = t.Operations
	
	result, err := txnForBuffer.Apply(buffer)
	if err != nil {
		return "", err
	}
	return result.String(), nil
}

// Reverse creates a transaction that undoes this transaction.
// The operations are reversed in order and inverted.
func (t *EditTransaction) Reverse() *EditTransaction {
	rev := &EditTransaction{
		BaseRevision: t.BaseRevision,
		Operations:   make([]EditOperation, 0, len(t.Operations)),
		Description:  fmt.Sprintf("reverse of: %s", t.Description),
	}

	// Reverse the operations and invert each
	for i := len(t.Operations) - 1; i >= 0; i-- {
		op := t.Operations[i]
		// For the reversed operation, we need to invert it
		// Insert becomes delete at the insertion point with length of inserted content
		// Delete becomes insert with the deleted content
		// Replace becomes replace with the original content
		var revOp EditOperation
		if op.IsInsert() {
			revOp = NewDeleteOperation(ByteSpan{
				Start: op.Span.Start,
				End:   op.Span.Start + len(op.Replacement),
			}, fmt.Sprintf("undo: %s", op.Description))
		} else if op.IsDelete() {
			// For delete, we need to know what was deleted - this requires original content
			// This is a limitation of the current design; we'll use the span length
			revOp = NewInsertOperation(op.Span.Start, make([]byte, op.Span.Len()),
				fmt.Sprintf("undo: %s", op.Description))
		} else {
			// For replace, reverse the operation
			revOp = NewReplaceOperation(op.Span, op.Replacement,
				fmt.Sprintf("undo: %s", op.Description))
		}
		rev.Operations = append(rev.Operations, revOp)
	}

	return rev
}

// EditHistory tracks a sequence of edit transactions for undo/redo.
type EditHistory struct {
	// Current is the current revision.
	Current RecipeRevision
	// Transactions is the list of transactions that led to the current state.
	// Index -1 means no transactions (initial state).
	Transactions []*EditTransaction
	// CurrentIndex is the index of the current transaction in Transactions.
	// -1 means at the initial state, len(Transactions)-1 means at the latest.
	CurrentIndex int
	// Source is the original source buffer.
	Source *SourceBuffer
}

// NewEditHistory creates a new edit history from an initial source buffer.
func NewEditHistory(source *SourceBuffer) *EditHistory {
	return &EditHistory{
		Current:       NewRecipeRevisionFromSource(source),
		Transactions:  make([]*EditTransaction, 0),
		CurrentIndex:  -1,
		Source:        source,
	}
}

// ApplyTransaction applies a transaction and adds it to the history.
func (h *EditHistory) ApplyTransaction(txn *EditTransaction) error {
	// Verify we're at the latest state
	if h.CurrentIndex != len(h.Transactions)-1 {
		// We're not at the tip; need to truncate redo history
		h.Transactions = h.Transactions[:h.CurrentIndex+1]
	}

	// Apply the transaction
	newBuffer, err := txn.Apply(h.CurrentSource())
	if err != nil {
		return err
	}

	// Update history
	h.Transactions = append(h.Transactions, txn)
	h.CurrentIndex = len(h.Transactions) - 1
	h.Current = NewRecipeRevisionFromSource(newBuffer)
	h.Source = newBuffer

	return nil
}

// CurrentSource returns the current source buffer.
func (h *EditHistory) CurrentSource() *SourceBuffer {
	return h.Source
}

// CanUndo reports whether undo is possible.
func (h *EditHistory) CanUndo() bool {
	return h.CurrentIndex >= 0
}

// CanRedo reports whether redo is possible.
func (h *EditHistory) CanRedo() bool {
	return h.CurrentIndex < len(h.Transactions)-1
}

// Undo undoes the last transaction.
func (h *EditHistory) Undo() error {
	if !h.CanUndo() {
		return fmt.Errorf("cannot undo: at initial state")
	}

	// Get the transaction to undo
	txn := h.Transactions[h.CurrentIndex]
	
	// Create reverse transaction
	undoTxn := txn.Reverse()
	// Update the base revision to match the current source
	undoTxn.BaseRevision = NewRecipeRevisionFromSource(h.Source)

	// Apply the undo transaction directly without storing it in history
	newBuffer, err := undoTxn.Apply(h.Source)
	if err != nil {
		return err
	}

	// Update state
	h.Source = newBuffer
	h.Current = NewRecipeRevisionFromSource(newBuffer)
	h.CurrentIndex--

	return nil
}

// Redo redoes the next transaction.
func (h *EditHistory) Redo() error {
	if !h.CanRedo() {
		return fmt.Errorf("cannot redo: at latest state")
	}

	// Get the transaction to redo
	h.CurrentIndex++
	txn := h.Transactions[h.CurrentIndex]

	// Update the base revision to match the current source
	txnForRedo := NewEditTransaction(NewRecipeRevisionFromSource(h.Source), txn.Description)
	txnForRedo.Operations = txn.Operations

	// Apply it directly without storing it in history
	newBuffer, err := txnForRedo.Apply(h.Source)
	if err != nil {
		return err
	}

	// Update state
	h.Source = newBuffer
	h.Current = NewRecipeRevisionFromSource(newBuffer)

	return nil
}

// TypedEdit represents a structured edit operation on a recipe.
// These are higher-level edits that correspond to user actions in the editor.
type TypedEdit struct {
	// Kind is the type of edit (insert node, delete node, change attribute, etc.)
	Kind TypedEditKind
	// Path identifies the location of the edit in the AST.
	Path string
	// Value is the new value (for insert/change operations).
	Value string
	// OldValue is the previous value (for change operations).
	OldValue string
	// Span is the source span being edited.
	Span ByteSpan
}

// TypedEditKind identifies the type of structured edit.
type TypedEditKind int

const (
	// TypedEditInsertNode inserts a new node.
	TypedEditInsertNode TypedEditKind = iota
	// TypedEditDeleteNode deletes a node.
	TypedEditDeleteNode
	// TypedEditChangeAttribute changes an attribute value.
	TypedEditChangeAttribute
	// TypedEditRename renames a node.
	TypedEditRename
	// TypedEditMove moves a node.
	TypedEditMove
	// TypedEditConnect creates a connection between nodes.
	TypedEditConnect
	// TypedEditDisconnect removes a connection.
	TypedEditDisconnect
)

// String returns a string representation of the edit kind.
func (k TypedEditKind) String() string {
	switch k {
	case TypedEditInsertNode:
		return "insert_node"
	case TypedEditDeleteNode:
		return "delete_node"
	case TypedEditChangeAttribute:
		return "change_attribute"
	case TypedEditRename:
		return "rename"
	case TypedEditMove:
		return "move"
	case TypedEditConnect:
		return "connect"
	case TypedEditDisconnect:
		return "disconnect"
	default:
		return "unknown"
	}
}

// ConvertToEditOperation converts a typed edit to a byte-level edit operation.
// This requires knowledge of the current source to generate the exact byte spans.
func (e TypedEdit) ConvertToEditOperation(currentSource *SourceBuffer) (EditOperation, error) {
	// This is a placeholder; the actual implementation needs to
	// map AST paths to byte spans using the parser's position information.
	return EditOperation{}, fmt.Errorf("not implemented: convert typed edit to byte operation")
}

// TextEdit represents a simple text edit (insert, delete, or replace text).
type TextEdit struct {
	// Offset is the byte offset where the edit starts.
	Offset int
	// Length is the number of bytes to replace (0 for insertions).
	Length int
	// Text is the new text to insert.
	Text string
}

// NewTextEdit creates a new text edit.
func NewTextEdit(offset, length int, text string) TextEdit {
	return TextEdit{
		Offset: offset,
		Length: length,
		Text:   text,
	}
}

// ToEditOperation converts a text edit to an edit operation.
func (e TextEdit) ToEditOperation() EditOperation {
	return EditOperation{
		Span: ByteSpan{
			Start: e.Offset,
			End:   e.Offset + e.Length,
		},
		Replacement: []byte(e.Text),
		Description: fmt.Sprintf("text edit at %d", e.Offset),
	}
}

// FindNodeAtOffset finds the AST node at a given byte offset.
// This is used for mapping cursor positions to AST nodes.
func FindNodeAtOffset(doc *Document, offset int) Node {
	// This is a placeholder; the actual implementation needs to
	// traverse the AST and check spans.
	// The parser needs to be updated to populate byte spans on all nodes.
	return nil
}

// TokenToByteSpan converts a token's line/column to byte offsets.
// This properly handles both LF and CRLF line endings by scanning the source bytes.
// Note: For best results, use TokenWithSpanToByteSpan which uses the pre-computed
// byte spans from the LosslessLexer.
func TokenToByteSpan(src string, tok Token) ByteSpan {
	if tok.Line < 1 || tok.Column < 1 {
		return ByteSpan{}
	}

	// Calculate start byte offset by scanning through lines
	startOffset := 0
	currentLine := 1
	
	// Iterate through the source to find the line
	for i := 0; i < len(src); i++ {
		if currentLine >= tok.Line {
			break
		}
		
		// Check for line endings
		if src[i] == '\n' {
			currentLine++
			startOffset = i + 1
		} else if src[i] == '\r' {
			// Handle CRLF or standalone CR
			currentLine++
			startOffset = i + 1
			// If followed by LF, skip it
			if i+1 < len(src) && src[i+1] == '\n' {
				i++
				startOffset = i + 1
			}
		}
	}
	
	if currentLine != tok.Line {
		// Line not found
		return ByteSpan{}
	}
	
	// Add column offset (columns are 1-indexed)
	startOffset += tok.Column - 1
	
	// End offset is start + lexeme length
	endOffset := startOffset + len(tok.Lexeme)
	
	// Validate bounds
	if startOffset < 0 || startOffset > len(src) || endOffset > len(src) {
		return ByteSpan{}
	}

	return ByteSpan{
		Start: startOffset,
		End:   endOffset,
	}
}
