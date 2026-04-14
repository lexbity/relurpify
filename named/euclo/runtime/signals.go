package runtime

import (
	"regexp"
	"strings"
)

// ClassificationSignal represents a single piece of evidence used during classification.
type ClassificationSignal struct {
	Kind   string  `json:"kind"`   // keyword, file_pattern, task_structure, error_text, context_hint, workspace_state
	Value  string  `json:"value"`  // the matched pattern or hint
	Weight float64 `json:"weight"` // contribution to score
	Mode   string  `json:"mode"`   // which mode this signal supports
}

// ModeCandidate is a scored mode recommendation.
type ModeCandidate struct {
	Mode    string   `json:"mode"`
	Score   float64  `json:"score"`
	Signals []string `json:"signals"`
}

// ScoredClassification extends TaskClassification with confidence scoring.
type ScoredClassification struct {
	TaskClassification
	Candidates []ModeCandidate        `json:"candidates"`
	Confidence float64                `json:"confidence"` // 0-1, highest candidate score normalized
	Ambiguous  bool                   `json:"ambiguous"`  // top two within threshold
	Signals    []ClassificationSignal `json:"signals"`
}

// AmbiguityThreshold is the score gap below which the top two modes
// are considered ambiguous. When gap < threshold, the classifier should
// ask the user rather than guessing.
const AmbiguityThreshold = 0.15

// Signal weights by kind.
const (
	WeightContextHint    = 1.0
	WeightKeyword        = 0.3
	WeightKeywordReview  = 0.45 // review signals need to beat the code default baseline
	WeightFilePattern    = 0.4
	WeightTaskStructure  = 0.5
	WeightErrorText      = 0.6
	WeightWorkspaceState = 0.35
	WeightDefault        = 0.4 // baseline for default mode when no strong signals fire
)

// ──────────────────────────────────────────────────────────────
// Signal collection
// ──────────────────────────────────────────────────────────────

// CollectSignals gathers all classification signals from the envelope.
func CollectSignals(envelope TaskEnvelope) []ClassificationSignal {
	var signals []ClassificationSignal
	signals = append(signals, collectContextHints(envelope)...)
	signals = append(signals, collectKeywordSignals(envelope.Instruction)...)
	signals = append(signals, collectFilePatternSignals(envelope)...)
	signals = append(signals, collectTaskStructureSignals(envelope.Instruction)...)
	signals = append(signals, collectErrorTextSignals(envelope.Instruction)...)
	signals = append(signals, collectWorkspaceStateSignals(envelope)...)
	return signals
}

// collectContextHints checks for explicit mode hints in task context.
func collectContextHints(envelope TaskEnvelope) []ClassificationSignal {
	var signals []ClassificationSignal
	if envelope.ModeHint != "" {
		signals = append(signals, ClassificationSignal{
			Kind:   "context_hint",
			Value:  "mode_hint:" + envelope.ModeHint,
			Weight: WeightContextHint,
			Mode:   envelope.ModeHint,
		})
	}
	if envelope.ResumedMode != "" {
		signals = append(signals, ClassificationSignal{
			Kind:   "context_hint",
			Value:  "resumed_mode:" + envelope.ResumedMode,
			Weight: WeightContextHint * 0.8,
			Mode:   envelope.ResumedMode,
		})
	}
	return signals
}

// collectKeywordSignals matches keywords in the instruction text.
func collectKeywordSignals(instruction string) []ClassificationSignal {
	lower := strings.ToLower(instruction)
	var signals []ClassificationSignal

	type keywordGroup struct {
		patterns []string
		mode     string
		weight   float64
	}
	groups := []keywordGroup{
		{[]string{"review", "audit", "inspect", "look at", "pull request", "pull-request", "code review", "lgtm", "compare", "contrast", "evaluate", "examine", "assess", "analyze", "analyse"}, "review", WeightKeywordReview},
		{[]string{"debug", "diagnose", "root cause", "failing", "failure", "trace"}, "debug", WeightKeyword},
		{[]string{"plan", "design", "architecture", "approach"}, "planning", WeightKeyword},
		{[]string{"test first", "tdd", "write tests", "add tests", "make sure it's tested", "ensure it's tested"}, "tdd", WeightKeyword},
		{[]string{"implement", "fix", "change", "refactor", "patch", "update", "add"}, "code", WeightKeyword},
		{[]string{"explain", "what is", "how do", "show me", "describe", "what does", "how can", "tell me", "help me understand", "walk me through"}, "chat", WeightKeyword},
	}

	for _, g := range groups {
		for _, p := range g.patterns {
			if strings.Contains(lower, p) {
				signals = append(signals, ClassificationSignal{
					Kind:   "keyword",
					Value:  p,
					Weight: g.weight,
					Mode:   g.mode,
				})
			}
		}
	}
	return signals
}

// collectFilePatternSignals looks at referenced files in the envelope.
func collectFilePatternSignals(envelope TaskEnvelope) []ClassificationSignal {
	var signals []ClassificationSignal
	lower := strings.ToLower(envelope.Instruction)

	// Test file patterns → support tdd mode.
	testPatterns := []string{"_test.go", ".test.", ".spec.", "_spec.", "test_", "tests/"}
	for _, p := range testPatterns {
		if strings.Contains(lower, p) {
			signals = append(signals, ClassificationSignal{
				Kind:   "file_pattern",
				Value:  p,
				Weight: WeightFilePattern,
				Mode:   "tdd",
			})
			break
		}
	}

	// Config/infra patterns → support planning mode.
	configPatterns := []string{".yaml", ".yml", "dockerfile", "makefile", ".toml", "go.mod"}
	for _, p := range configPatterns {
		if strings.Contains(lower, p) {
			signals = append(signals, ClassificationSignal{
				Kind:   "file_pattern",
				Value:  p,
				Weight: WeightFilePattern * 0.5,
				Mode:   "planning",
			})
			break
		}
	}

	return signals
}

var (
	taskStructureDebugPatterns = []struct {
		pattern *regexp.Regexp
		value   string
	}{
		{regexp.MustCompile(`(?i)\bused to work\b`), "used_to_work"},
		{regexp.MustCompile(`(?i)\bused to pass\b`), "used_to_pass"},
		{regexp.MustCompile(`(?i)\bbroke\b`), "broke"},
		{regexp.MustCompile(`(?i)\bregression\b`), "regression"},
		{regexp.MustCompile(`(?i)\bstopped working\b`), "stopped_working"},
		{regexp.MustCompile(`(?i)\bno longer\b`), "no_longer"},
		{regexp.MustCompile(`(?i)\bwhy (does|is|did)\b`), "why_question"},
	}

	taskStructurePlanningPatterns = []struct {
		pattern *regexp.Regexp
		value   string
	}{
		{regexp.MustCompile(`(?i)\bhow (should|would|can) (I|we)\b`), "how_should"},
		{regexp.MustCompile(`(?i)\bstrategy\b`), "strategy"},
		{regexp.MustCompile(`(?i)\bmigrat(e|ion)\b`), "migration"},
		{regexp.MustCompile(`(?i)\brewrite\b`), "rewrite"},
		{regexp.MustCompile(`(?i)\bredesign\b`), "redesign"},
		{regexp.MustCompile(`(?i)\bacross multiple\b`), "across_multiple"},
	}

	taskStructureSimpleRepairPatterns = []struct {
		pattern *regexp.Regexp
		value   string
	}{
		{regexp.MustCompile(`(?i)^fix\s*[:\-]\s*`), "fix_prefix"},
		{regexp.MustCompile(`(?i)^fix this\b`), "fix_this"},
		{regexp.MustCompile(`(?i)\bapply a fix\b`), "apply_fix"},
		{regexp.MustCompile(`(?i)\bquick (fix|repair)\b`), "quick_repair"},
		{regexp.MustCompile(`(?i)\bsimple fix\b`), "simple_fix"},
		{regexp.MustCompile(`(?i)\bminor fix\b`), "minor_fix"},
		{regexp.MustCompile(`(?i)\bsmall fix\b`), "small_fix"},
		{regexp.MustCompile(`(?i)\bpatch this\b`), "patch_this"},
		{regexp.MustCompile(`(?i)\bfix the bug\b`), "fix_bug"},
		{regexp.MustCompile(`(?i)\bfix it\b`), "fix_it"},
	}
)

// collectTaskStructureSignals detects structural patterns in the instruction.
func collectTaskStructureSignals(instruction string) []ClassificationSignal {
	var signals []ClassificationSignal

	for _, p := range taskStructureDebugPatterns {
		if p.pattern.MatchString(instruction) {
			signals = append(signals, ClassificationSignal{
				Kind:   "task_structure",
				Value:  p.value,
				Weight: WeightTaskStructure,
				Mode:   "debug",
			})
		}
	}

	for _, p := range taskStructurePlanningPatterns {
		if p.pattern.MatchString(instruction) {
			signals = append(signals, ClassificationSignal{
				Kind:   "task_structure",
				Value:  p.value,
				Weight: WeightTaskStructure,
				Mode:   "planning",
			})
		}
	}

	// Simple repair patterns indicate a direct fix request without investigation.
	// These are used for sub-capability selection within debug mode.
	for _, p := range taskStructureSimpleRepairPatterns {
		if p.pattern.MatchString(instruction) {
			signals = append(signals, ClassificationSignal{
				Kind:   "task_structure",
				Value:  "simple_repair:" + p.value,
				Weight: WeightTaskStructure,
				Mode:   "debug",
			})
		}
	}

	return signals
}

var errorTextPatterns = []struct {
	pattern *regexp.Regexp
	value   string
}{
	{regexp.MustCompile(`(?i)\bpanic\b`), "panic"},
	{regexp.MustCompile(`(?i)\bstacktrace\b`), "stacktrace"},
	{regexp.MustCompile(`(?i)\bstack trace\b`), "stack_trace"},
	{regexp.MustCompile(`(?i)\bgoroutine \d+`), "goroutine_dump"},
	{regexp.MustCompile(`(?i)\berror:\s`), "error_prefix"},
	{regexp.MustCompile(`(?i)\bfatal\b`), "fatal"},
	{regexp.MustCompile(`(?i)\bnil pointer\b`), "nil_pointer"},
	{regexp.MustCompile(`(?i)\bsegfault\b`), "segfault"},
	{regexp.MustCompile(`(?i)\bexit code \d+`), "exit_code"},
	{regexp.MustCompile(`\.go:\d+`), "go_file_line"},
}

// collectErrorTextSignals detects error messages, stack traces, and panic text.
func collectErrorTextSignals(instruction string) []ClassificationSignal {
	var signals []ClassificationSignal
	for _, p := range errorTextPatterns {
		if p.pattern.MatchString(instruction) {
			signals = append(signals, ClassificationSignal{
				Kind:   "error_text",
				Value:  p.value,
				Weight: WeightErrorText,
				Mode:   "debug",
			})
		}
	}
	return signals
}

// collectWorkspaceStateSignals generates signals from prior artifacts and
// capability state in the envelope.
func collectWorkspaceStateSignals(envelope TaskEnvelope) []ClassificationSignal {
	var signals []ClassificationSignal

	for _, kind := range envelope.PreviousArtifactKinds {
		switch {
		case strings.Contains(kind, "verification") || strings.Contains(kind, "test"):
			signals = append(signals, ClassificationSignal{
				Kind:   "workspace_state",
				Value:  "has_verification_artifact",
				Weight: WeightWorkspaceState,
				Mode:   "code", // verification exists → continue coding
			})
		case strings.Contains(kind, "plan"):
			signals = append(signals, ClassificationSignal{
				Kind:   "workspace_state",
				Value:  "has_plan_artifact",
				Weight: WeightWorkspaceState,
				Mode:   "code", // plan exists → execute it
			})
		case strings.Contains(kind, "explore") || strings.Contains(kind, "analyze"):
			signals = append(signals, ClassificationSignal{
				Kind:   "workspace_state",
				Value:  "has_explore_artifact",
				Weight: WeightWorkspaceState * 0.5,
				Mode:   "code",
			})
		}
	}

	if !envelope.EditPermitted {
		signals = append(signals, ClassificationSignal{
			Kind:   "workspace_state",
			Value:  "read_only",
			Weight: WeightWorkspaceState,
			Mode:   "review",
		})
	}

	return signals
}

// ──────────────────────────────────────────────────────────────
// Score aggregation
// ──────────────────────────────────────────────────────────────

// ScoreSignals aggregates signals into ranked mode candidates.
func ScoreSignals(signals []ClassificationSignal) []ModeCandidate {
	scores := map[string]float64{}
	signalNames := map[string][]string{}

	for _, s := range signals {
		scores[s.Mode] += s.Weight
		signalNames[s.Mode] = append(signalNames[s.Mode], s.Kind+":"+s.Value)
	}

	var candidates []ModeCandidate
	for mode, score := range scores {
		candidates = append(candidates, ModeCandidate{
			Mode:    mode,
			Score:   score,
			Signals: signalNames[mode],
		})
	}

	// Sort by score descending.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].Score > candidates[j-1].Score; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}

	return candidates
}

// NormalizeConfidence converts the top score to a 0-1 range.
// Uses the total signal weight as the maximum possible score.
func NormalizeConfidence(candidates []ModeCandidate, totalWeight float64) float64 {
	if len(candidates) == 0 || totalWeight <= 0 {
		return 0
	}
	return candidates[0].Score / totalWeight
}

// IsAmbiguous returns true when the top two candidates are within the threshold.
func IsAmbiguous(candidates []ModeCandidate) bool {
	if len(candidates) < 2 {
		return false
	}
	if candidates[0].Score <= 0 {
		return true
	}
	gap := (candidates[0].Score - candidates[1].Score) / candidates[0].Score
	return gap < AmbiguityThreshold
}
