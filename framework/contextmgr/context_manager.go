// Package contextmgr manages context items within LLM token budgets.
// It provides pluggable pruning strategies (Progressive, Conservative, Aggressive)
// and a ProgressiveLoader for lazy file loading, ensuring agents stay within model
// context limits while retaining the most relevant information.
package contextmgr

import (
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"sync"
)

// ContextManager orchestrates context items within a budget.
type ContextManager struct {
	mu            sync.RWMutex
	budget        *ContextBudget
	items         []ContextItem
	strategy      PruningStrategy
	filePathIndex map[string]int // path → index into items; rebuilt after pruning/compression
	totalTokens   int
	itemsByType   map[ContextItemType]int
}

// PruningStrategy defines selection rules for compression/pruning.
type PruningStrategy interface {
	SelectForPruning(items []ContextItem, targetTokens int) []ContextItem
	SelectForCompression(items []ContextItem, targetTokens int) []ContextItem
}

// NewContextManager builds a manager with the default strategy.
func NewContextManager(budget *ContextBudget) *ContextManager {
	return &ContextManager{
		budget:        budget,
		items:         make([]ContextItem, 0),
		strategy:      NewRelevanceBasedStrategy(),
		filePathIndex: make(map[string]int),
		itemsByType:   make(map[ContextItemType]int),
	}
}

// AddItem registers a new context item, enforcing the budget.
func (cm *ContextManager) AddItem(item ContextItem) error {
	if item == nil {
		return fmt.Errorf("nil context item")
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if !cm.budget.CanAddTokens(item.TokenCount()) {
		if err := cm.makeSpaceLocked(item.TokenCount()); err != nil {
			return fmt.Errorf("cannot add item: %w", err)
		}
	}
	cm.items = append(cm.items, item)
	cm.addAggregateLocked(item)
	cm.syncBudgetLocked()
	return nil
}

// UpsertFileItem inserts or replaces a file context entry keyed by path.
func (cm *ContextManager) UpsertFileItem(item *core.FileContextItem) error {
	if item == nil {
		return fmt.Errorf("nil file context item")
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()

	existingTokens := 0
	index, exists := cm.filePathIndex[item.Path]
	if exists {
		existingTokens = cm.items[index].TokenCount()
	}

	delta := item.TokenCount() - existingTokens
	if delta > 0 && !cm.budget.CanAddTokens(delta) {
		if err := cm.makeSpaceLocked(delta); err != nil {
			return fmt.Errorf("cannot upsert file item: %w", err)
		}
		// Pruning may have shifted the index; look it up again.
		index, exists = cm.filePathIndex[item.Path]
	}

	if exists {
		cm.replaceAggregateLocked(cm.items[index], item)
		cm.items[index] = item
	} else {
		cm.filePathIndex[item.Path] = len(cm.items)
		cm.items = append(cm.items, item)
		cm.addAggregateLocked(item)
	}
	cm.syncBudgetLocked()
	return nil
}

// makeSpace frees tokens via compression or pruning.
func (cm *ContextManager) makeSpace(neededTokens int) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.makeSpaceLocked(neededTokens)
}

// MakeSpace is the exported wrapper for freeing capacity.
func (cm *ContextManager) MakeSpace(tokens int) error {
	return cm.makeSpace(tokens)
}

func (cm *ContextManager) makeSpaceLocked(neededTokens int) error {
	state := cm.budget.CheckBudget()
	switch state {
	case BudgetNeedsCompression:
		return cm.compressItemsLocked(neededTokens)
	case BudgetCritical:
		if err := cm.compressItemsLocked(neededTokens); err == nil {
			return nil
		}
		return cm.pruneItemsLocked(neededTokens)
	default:
		return fmt.Errorf("insufficient budget")
	}
}

func (cm *ContextManager) compressItemsLocked(targetTokens int) error {
	toCompress := cm.strategy.SelectForCompression(cm.items, targetTokens)
	if len(toCompress) == 0 {
		return fmt.Errorf("no items available for compression")
	}
	freed := 0
	replacements := make(map[ContextItem]ContextItem)
	for _, item := range toCompress {
		compressed, err := item.Compress()
		if err != nil {
			continue
		}
		freed += item.TokenCount() - compressed.TokenCount()
		replacements[item] = compressed
		if freed >= targetTokens {
			break
		}
	}
	if freed < targetTokens {
		return fmt.Errorf("compression freed only %d tokens, needed %d", freed, targetTokens)
	}
	cm.replaceItemsLocked(replacements)
	cm.rebuildFilePathIndexLocked()
	cm.syncBudgetLocked()
	return nil
}

func (cm *ContextManager) pruneItemsLocked(targetTokens int) error {
	toPrune := cm.strategy.SelectForPruning(cm.items, targetTokens)
	if len(toPrune) == 0 {
		return fmt.Errorf("no items available for pruning")
	}
	freed := 0
	removeSet := make(map[ContextItem]struct{})
	for _, item := range toPrune {
		freed += item.TokenCount()
		removeSet[item] = struct{}{}
	}
	if freed < targetTokens {
		return fmt.Errorf("pruning would free only %d tokens, needed %d", freed, targetTokens)
	}
	filtered := make([]ContextItem, 0, len(cm.items))
	for _, item := range cm.items {
		if _, remove := removeSet[item]; !remove {
			filtered = append(filtered, item)
			continue
		}
		cm.removeAggregateLocked(item)
	}
	cm.items = filtered
	cm.rebuildFilePathIndexLocked()
	cm.syncBudgetLocked()
	return nil
}

// GetItems returns all items tracked by the manager.
func (cm *ContextManager) GetItems() []ContextItem {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return append([]ContextItem(nil), cm.items...)
}

// GetItemsByType returns the subset of items matching the provided type.
func (cm *ContextManager) GetItemsByType(t ContextItemType) []ContextItem {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make([]ContextItem, 0)
	for _, item := range cm.items {
		if item.Type() == t {
			result = append(result, item)
		}
	}
	return result
}

// FileItems returns the managed file context entries.
func (cm *ContextManager) FileItems() []*core.FileContextItem {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make([]*core.FileContextItem, 0)
	for _, item := range cm.items {
		file, ok := item.(*core.FileContextItem)
		if !ok {
			continue
		}
		result = append(result, file)
	}
	return result
}

// Clear removes all context items.
func (cm *ContextManager) Clear() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.items = make([]ContextItem, 0)
	cm.filePathIndex = make(map[string]int)
	cm.totalTokens = 0
	cm.itemsByType = make(map[ContextItemType]int)
	cm.syncBudgetLocked()
}

// GetStats reports aggregated item/budget information.
func (cm *ContextManager) GetStats() ContextStats {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	stats := ContextStats{
		TotalItems:  len(cm.items),
		TotalTokens: cm.totalTokens,
		ItemsByType: cloneContextItemCounts(cm.itemsByType),
	}
	stats.BudgetUsage = cm.budget.GetCurrentUsage()
	stats.BudgetState = cm.budget.CheckBudget()
	return stats
}

// ContextStats captures context management metrics.
type ContextStats struct {
	TotalItems  int
	TotalTokens int
	ItemsByType map[ContextItemType]int
	BudgetUsage TokenUsage
	BudgetState BudgetState
}

func (cm *ContextManager) syncBudgetLocked() {
	usage := cm.budget.GetCurrentUsage()
	usage.ContextTokens = cm.totalTokens
	usage.TotalTokens = usage.SystemTokens + usage.ToolTokens + cm.totalTokens + usage.OutputTokens
	if cm.budget.AvailableForContext > 0 {
		usage.ContextUsagePercent = float64(cm.totalTokens) / float64(cm.budget.AvailableForContext)
	}
	cm.budget.SetCurrentUsage(usage)
}

// rebuildFilePathIndexLocked reconstructs filePathIndex from the current items slice.
// Called after pruning or compression, which can remove or reorder file items.
// Must be called with cm.mu held for write.
func (cm *ContextManager) rebuildFilePathIndexLocked() {
	clear(cm.filePathIndex)
	for i, item := range cm.items {
		if f, ok := item.(*core.FileContextItem); ok {
			cm.filePathIndex[f.Path] = i
		}
	}
}

func (cm *ContextManager) replaceItemsLocked(replacements map[ContextItem]ContextItem) {
	replaced := make([]ContextItem, 0, len(cm.items))
	for _, item := range cm.items {
		if replacement, ok := replacements[item]; ok {
			cm.replaceAggregateLocked(item, replacement)
			replaced = append(replaced, replacement)
		} else {
			replaced = append(replaced, item)
		}
	}
	cm.items = replaced
}

func (cm *ContextManager) addAggregateLocked(item ContextItem) {
	if item == nil {
		return
	}
	cm.totalTokens += item.TokenCount()
	cm.itemsByType[item.Type()]++
}

func (cm *ContextManager) removeAggregateLocked(item ContextItem) {
	if item == nil {
		return
	}
	cm.totalTokens -= item.TokenCount()
	if cm.totalTokens < 0 {
		cm.totalTokens = 0
	}
	itemType := item.Type()
	if count := cm.itemsByType[itemType]; count <= 1 {
		delete(cm.itemsByType, itemType)
	} else {
		cm.itemsByType[itemType] = count - 1
	}
}

func (cm *ContextManager) replaceAggregateLocked(oldItem, newItem ContextItem) {
	if oldItem == nil {
		cm.addAggregateLocked(newItem)
		return
	}
	if newItem == nil {
		cm.removeAggregateLocked(oldItem)
		return
	}
	cm.totalTokens += newItem.TokenCount() - oldItem.TokenCount()
	oldType := oldItem.Type()
	newType := newItem.Type()
	if oldType != newType {
		if count := cm.itemsByType[oldType]; count <= 1 {
			delete(cm.itemsByType, oldType)
		} else {
			cm.itemsByType[oldType] = count - 1
		}
		cm.itemsByType[newType]++
	}
}

func cloneContextItemCounts(input map[ContextItemType]int) map[ContextItemType]int {
	if len(input) == 0 {
		return make(map[ContextItemType]int)
	}
	out := make(map[ContextItemType]int, len(input))
	for kind, count := range input {
		out[kind] = count
	}
	return out
}
