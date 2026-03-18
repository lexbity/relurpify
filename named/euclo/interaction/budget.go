package interaction

import (
	"sync"
	"time"
)

// InteractionBudget enforces limits on interaction-heavy sessions.
// When limits are hit, the PhaseMachine auto-advances with defaults.
type InteractionBudget struct {
	MaxQuestionsPerPhase int           // max questions within a single phase (default: 3)
	MaxPhasesSkippable   int           // max phases that can be skipped (0 = unlimited)
	AutoAdvanceAfter     time.Duration // auto-advance if no response within this duration (0 = disabled)
	MaxTransitions       int           // max mode transitions (default: 3)
	MaxFramesTotal       int           // max total frames emitted (0 = unlimited)

	mu               sync.Mutex
	questionsInPhase map[string]int
	skippedCount     int
	transitionCount  int
	frameCount       int
}

// DefaultBudget returns an InteractionBudget with sensible defaults.
func DefaultBudget() *InteractionBudget {
	return &InteractionBudget{
		MaxQuestionsPerPhase: 3,
		MaxPhasesSkippable:   0, // unlimited
		AutoAdvanceAfter:     0, // disabled
		MaxTransitions:       3,
		MaxFramesTotal:       0, // unlimited
		questionsInPhase:     make(map[string]int),
	}
}

// NewBudget creates a budget from an InteractionConfig.
func NewBudget(cfg InteractionConfig) *InteractionBudget {
	b := DefaultBudget()
	if cfg.Budget.MaxQuestions > 0 {
		b.MaxQuestionsPerPhase = cfg.Budget.MaxQuestions
	}
	if cfg.Budget.MaxTransitions > 0 {
		b.MaxTransitions = cfg.Budget.MaxTransitions
	}
	if cfg.Budget.MaxFrames > 0 {
		b.MaxFramesTotal = cfg.Budget.MaxFrames
	}
	return b
}

// RecordFrame increments the frame counter. Returns true if within budget.
func (b *InteractionBudget) RecordFrame() bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.frameCount++
	if b.MaxFramesTotal > 0 && b.frameCount > b.MaxFramesTotal {
		return false
	}
	return true
}

// RecordQuestion increments the question counter for a phase. Returns true if within budget.
func (b *InteractionBudget) RecordQuestion(phase string) bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.questionsInPhase == nil {
		b.questionsInPhase = make(map[string]int)
	}
	b.questionsInPhase[phase]++
	if b.MaxQuestionsPerPhase > 0 && b.questionsInPhase[phase] > b.MaxQuestionsPerPhase {
		return false
	}
	return true
}

// RecordSkip increments the skip counter. Returns true if within budget.
func (b *InteractionBudget) RecordSkip() bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.skippedCount++
	if b.MaxPhasesSkippable > 0 && b.skippedCount > b.MaxPhasesSkippable {
		return false
	}
	return true
}

// RecordTransition increments the transition counter. Returns true if within budget.
func (b *InteractionBudget) RecordTransition() bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.transitionCount++
	return b.transitionCount <= b.MaxTransitions
}

// FrameCount returns the current frame count.
func (b *InteractionBudget) FrameCount() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.frameCount
}

// TransitionCount returns the current transition count.
func (b *InteractionBudget) TransitionCount() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.transitionCount
}

// ExhaustedReason returns a non-empty string describing why the budget is exhausted,
// or empty if still within limits.
func (b *InteractionBudget) ExhaustedReason() string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.MaxFramesTotal > 0 && b.frameCount > b.MaxFramesTotal {
		return "frame_budget_exceeded"
	}
	if b.MaxTransitions > 0 && b.transitionCount > b.MaxTransitions {
		return "transition_budget_exceeded"
	}
	return ""
}
