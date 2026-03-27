package context

import (
	"fmt"
	"sync"
)

// SimpleBudgetTracker is a lightweight token budget tracker for Phase 3.
//
// Phase 3 stub: In Phase 4+, this will integrate with framework/core.ContextBudget
// and contextmgr compression strategies for full token management.
//
// For now, it provides:
//   - Simple token accounting per category
//   - Warning thresholds (e.g., 80% of limit)
//   - Listener notifications for budget events
//   - Integration hooks for compression
type SimpleBudgetTracker struct {
	mu               sync.RWMutex
	totalTokens      int
	usedTokens       int
	categories       map[string]int // category name → tokens used
	warningPercent   int            // e.g., 80 (80% = warning threshold)
	listeners        []BudgetListener
	warningTriggered bool
}

// BudgetListener is notified of budget events.
type BudgetListener interface {
	OnBudgetWarning(remaining int, limit int) error
	OnBudgetExceeded(remaining int, limit int) error
}

// NewBudgetManager creates a lightweight budget tracker with a token limit.
func NewBudgetManager(tokenLimit int) *SimpleBudgetTracker {
	return &SimpleBudgetTracker{
		totalTokens:    tokenLimit,
		usedTokens:     0,
		categories:     make(map[string]int),
		warningPercent: 80,
		listeners:      make([]BudgetListener, 0),
	}
}

// Budget returns current budget metrics (simplified).
func (m *SimpleBudgetTracker) Budget() map[string]interface{} {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"total":     m.totalTokens,
		"used":      m.usedTokens,
		"remaining": m.totalTokens - m.usedTokens,
		"percent":   (m.usedTokens * 100) / m.totalTokens,
	}
}

// Track records token usage in a category.
func (m *SimpleBudgetTracker) Track(category string, tokens int) error {
	if m == nil {
		return fmt.Errorf("budget tracker not initialized")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.usedTokens += tokens
	m.categories[category] += tokens

	// Check budget thresholds
	percent := (m.usedTokens * 100) / m.totalTokens
	remaining := m.totalTokens - m.usedTokens

	// Check exceeded first (critical)
	if percent >= 100 {
		m.notifyExceeded(remaining)
		return fmt.Errorf("budget exceeded: %d/%d tokens used", m.usedTokens, m.totalTokens)
	}

	// Check warning threshold
	if percent >= m.warningPercent && !m.warningTriggered {
		m.warningTriggered = true
		m.notifyWarning(remaining)
	}

	return nil
}

// Reset clears all tracked usage.
func (m *SimpleBudgetTracker) Reset() error {
	if m == nil {
		return fmt.Errorf("budget tracker not initialized")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.usedTokens = 0
	m.categories = make(map[string]int)
	m.warningTriggered = false
	return nil
}

// SetWarningThreshold sets the percentage at which warnings are triggered.
func (m *SimpleBudgetTracker) SetWarningThreshold(percent int) error {
	if percent < 1 || percent > 99 {
		return fmt.Errorf("warning threshold must be 1-99, got %d", percent)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.warningPercent = percent
	return nil
}

// AddListener registers a listener for budget events.
func (m *SimpleBudgetTracker) AddListener(listener BudgetListener) error {
	if listener == nil {
		return fmt.Errorf("listener required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, listener)
	return nil
}

// RemoveAllListeners clears all registered listeners.
func (m *SimpleBudgetTracker) RemoveAllListeners() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = make([]BudgetListener, 0)
}

// EstimatedCompression estimates token reduction from compression (50% rate).
func (m *SimpleBudgetTracker) EstimatedCompression() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Estimate 50% compression rate
	return m.usedTokens / 2
}

// Helper functions

func (m *SimpleBudgetTracker) notifyWarning(remaining int) {
	for _, listener := range m.listeners {
		_ = listener.OnBudgetWarning(remaining, m.totalTokens)
	}
}

func (m *SimpleBudgetTracker) notifyExceeded(remaining int) {
	for _, listener := range m.listeners {
		_ = listener.OnBudgetExceeded(remaining, m.totalTokens)
	}
}
