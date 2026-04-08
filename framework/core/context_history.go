package core

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// AddInteraction appends to the conversation history.
func (c *Context) AddInteraction(role, content string, metadata map[string]interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureHistoryWritableLocked()
	id := c.interactionIDCtr
	c.interactionIDCtr++
	c.history = append(c.history, Interaction{
		ID:        id,
		Role:      role,
		Content:   content,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	})
	c.historyDirty = true
	c.smartTruncateHistoryLocked()
}

// History returns the accumulated conversation history.
func (c *Context) History() []Interaction {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Interaction(nil), c.history...)
}

// TrimHistory keeps only the most recent interactions.
func (c *Context) TrimHistory(keep int) {
	if keep <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.history) <= keep {
		return
	}
	c.ensureHistoryWritableLocked()
	start := len(c.history) - keep
	c.history = append([]Interaction(nil), c.history[start:]...)
	c.historyDirty = true
}

// LatestInteraction returns the most recent interaction if any.
func (c *Context) LatestInteraction() (Interaction, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.history) == 0 {
		return Interaction{}, false
	}
	return c.history[len(c.history)-1], true
}

// smartTruncateHistoryLocked keeps the conversation history bounded while still
// preserving the very first message (usually the task instruction). The oldest
// middle portion is dropped so that downstream reasoning retains enough
// context without exhausting memory.
func (c *Context) smartTruncateHistoryLocked() {
	if len(c.history) <= c.maxHistory {
		return
	}
	start := len(c.history) - c.maxHistory
	c.history = append(c.history[:1], c.history[start:]...)
}

// SetKnowledge stores derived information available to all nodes.
func (c *Context) SetKnowledge(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.knowledge[key] = value
	c.dirtyKnowledge[key] = struct{}{}
}

// GetKnowledge retrieves derived info.
func (c *Context) GetKnowledge(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	val, ok := c.knowledge[key]
	if ok {
		return val, true
	}
	if c.parentKnowledge != nil {
		val, ok = c.parentKnowledge[key]
		if ok {
			val = deepCopyValue(val)
			c.knowledge[key] = val
		}
	}
	return val, ok
}

// CompressHistory summarizes older interactions while keeping the recent tail.
func (c *Context) CompressHistory(keepRecentCount int, llm LanguageModel, strategy CompressionStrategy) error {
	if strategy == nil {
		return fmt.Errorf("compression strategy required")
	}
	c.mu.RLock()
	if len(c.history) <= keepRecentCount {
		c.mu.RUnlock()
		return nil
	}
	compressibleCount := len(c.history) - keepRecentCount
	toCompress := append([]Interaction(nil), c.history[:compressibleCount]...)
	startID := toCompress[0].ID
	endID := toCompress[len(toCompress)-1].ID
	c.mu.RUnlock()

	compressed, err := strategy.Compress(toCompress, llm)
	if err != nil {
		return err
	}
	if compressed.OriginalTokens == 0 {
		compressed.OriginalTokens = estimateTokens(toCompress)
	}
	if compressed.CompressedTokens == 0 {
		compressed.CompressedTokens = strategy.EstimateTokens(compressed)
	}
	compressed.StartInteractionID = startID
	compressed.EndInteractionID = endID

	event := CompressionEvent{
		Timestamp:               time.Now().UTC(),
		InteractionsCompressed:  len(toCompress),
		TokensSaved:             compressed.OriginalTokens - compressed.CompressedTokens,
		CompressionMethod:       fmt.Sprintf("%T", strategy),
		StartInteractionID:      startID,
		EndInteractionID:        endID,
		CompressedSummaryTokens: compressed.CompressedTokens,
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if compressibleCount > len(c.history) {
		compressibleCount = len(c.history)
	}
	c.ensureHistoryWritableLocked()
	c.ensureCompressedWritableLocked()
	c.ensureLogWritableLocked()
	c.history = append([]Interaction(nil), c.history[compressibleCount:]...)
	c.compressedHistory = append(c.compressedHistory, *compressed)
	c.compressionLog = append(c.compressionLog, event)
	c.historyDirty = true
	c.compressedDirty = true
	c.compressionDirty = true
	return nil
}

// GetFullHistory returns compressed segments plus current tail.
func (c *Context) GetFullHistory() ([]CompressedContext, []Interaction) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]CompressedContext(nil), c.compressedHistory...), append([]Interaction(nil), c.history...)
}

// AppendCompressedContext appends a compressed history entry.
func (c *Context) AppendCompressedContext(cc CompressedContext) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureCompressedWritableLocked()
	c.compressedHistory = append(c.compressedHistory, cc)
	c.compressedDirty = true
}

// GetContextForLLM renders the context as a string suitable for prompts.
func (c *Context) GetContextForLLM() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var sb strings.Builder
	if len(c.compressedHistory) > 0 {
		sb.WriteString("=== Previous Context (Compressed) ===\n")
		for _, cc := range c.compressedHistory {
			sb.WriteString(fmt.Sprintf("Summary: %s\n", cc.Summary))
			sb.WriteString("Key Facts:\n")
			for _, fact := range cc.KeyFacts {
				sb.WriteString(fmt.Sprintf("  - [%s] %s\n", fact.Type, fact.Content))
			}
			sb.WriteString("\n")
		}
	}
	if len(c.history) > 0 {
		sb.WriteString("=== Recent Interactions ===\n")
		for _, interaction := range c.history {
			sb.WriteString(fmt.Sprintf("[%s]: %s\n", interaction.Role, interaction.Content))
		}
	}
	return sb.String()
}

// GetCompressionStats aggregates compression metrics.
func (c *Context) GetCompressionStats() CompressionStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	totalInteractions := 0
	totalTokensSaved := 0
	for _, event := range c.compressionLog {
		totalInteractions += event.InteractionsCompressed
		totalTokensSaved += event.TokensSaved
	}
	return CompressionStats{
		TotalInteractionsCompressed: totalInteractions,
		TotalTokensSaved:            totalTokensSaved,
		CompressionEvents:           len(c.compressionLog),
		CurrentHistorySize:          len(c.history),
		CompressedChunks:            len(c.compressedHistory),
	}
}

// CompressionStats summarizes compression activity.
type CompressionStats struct {
	TotalInteractionsCompressed int
	TotalTokensSaved            int
	CompressionEvents           int
	CurrentHistorySize          int
	CompressedChunks            int
}

// recordMergeConflict records a merge conflict for later inspection.
func (c *Context) recordMergeConflict(key string, losingValue, winningValue interface{}, area string) {
	record := MergeConflictRecord{
		Key:             key,
		ConflictArea:    area,
		LosingValueHash: hashValue(losingValue),
		Timestamp:       time.Now().UTC(),
	}
	c.mergeConflicts = append(c.mergeConflicts, record)
}

// MergeConflicts returns all recorded merge conflicts.
func (c *Context) MergeConflicts() []MergeConflictRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.mergeConflicts) == 0 {
		return nil
	}
	conflicts := make([]MergeConflictRecord, len(c.mergeConflicts))
	copy(conflicts, c.mergeConflicts)
	return conflicts
}

// hashValue computes a simple hash of a value for conflict logging.
func hashValue(v interface{}) string {
	if v == nil {
		return "nil"
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("unhashable:%T", v)
	}
	s := string(data)
	if len(s) > 16 {
		return s[:16]
	}
	return s
}

// deepEqual checks if two values are deeply equal.
func deepEqual(a, b interface{}) bool {
	return reflect.DeepEqual(a, b)
}
