package agenttest

import (
	"fmt"
	"strings"
)

// ToolDependency defines a prerequisite relationship between tools
type ToolDependency struct {
	Tool     string   `yaml:"tool"`               // The tool that has prerequisites
	Requires []string `yaml:"requires,omitempty"` // Tools that must precede it
	Excludes []string `yaml:"excludes,omitempty"` // Tools that must NOT be in same phase
}

// DependencyValidator checks tool sequences against dependency rules
type DependencyValidator struct {
	Rules []ToolDependency
}

// NewDependencyValidator creates a validator with the given rules
func NewDependencyValidator(rules []ToolDependency) *DependencyValidator {
	return &DependencyValidator{Rules: rules}
}

// Validate checks a tool transcript against dependency rules
func (v *DependencyValidator) Validate(transcript *ToolTranscriptArtifact) []string {
	var failures []string

	if transcript == nil || len(v.Rules) == 0 {
		return nil
	}

	// Build a map of tool indices for quick lookup
	toolIndices := make(map[string][]int)
	for i, entry := range transcript.Entries {
		tool := strings.TrimSpace(entry.Tool)
		if tool != "" {
			toolIndices[tool] = append(toolIndices[tool], i)
		}
	}

	// Check each rule against the transcript
	for _, rule := range v.Rules {
		ruleTool := strings.TrimSpace(rule.Tool)
		if ruleTool == "" {
			continue
		}

		// Get all occurrences of this tool
		occurrences, ok := toolIndices[ruleTool]
		if !ok {
			continue // Tool not used, no violations possible
		}

		// Check each occurrence
		for _, occurrence := range occurrences {
			if violations := v.checkRuleAt(rule, transcript, toolIndices, occurrence); len(violations) > 0 {
				failures = append(failures, violations...)
			}
		}
	}

	return failures
}

// checkRuleAt checks a rule at a specific tool occurrence
func (v *DependencyValidator) checkRuleAt(rule ToolDependency, transcript *ToolTranscriptArtifact, toolIndices map[string][]int, occurrence int) []string {
	var failures []string

	// Check "requires" constraints - tools that must have been called before
	// This is OR logic: any one of the requires satisfies the constraint
	if len(rule.Requires) > 0 {
		anyFound := false
		missingReqs := []string{}

		for _, req := range rule.Requires {
			req = strings.TrimSpace(req)
			if req == "" {
				continue
			}

			// Find the most recent occurrence of required tool before this occurrence
			reqIndices := toolIndices[req]
			found := false
			for _, idx := range reqIndices {
				if idx < occurrence {
					found = true
					break
				}
			}

			if found {
				anyFound = true
				break // OR logic: one satisfied is enough
			} else {
				missingReqs = append(missingReqs, req)
			}
		}

		if !anyFound && len(missingReqs) > 0 {
			failures = append(failures, fmt.Sprintf("dependency violation: %s (at index %d) requires one of [%s] but none found",
				rule.Tool, occurrence, strings.Join(missingReqs, ", ")))
		}
	}

	// Check "excludes" constraints - tools that must not be in the same phase
	// "Phase" is defined as the segment between two checkpoint/cycle boundaries
	// For simplicity, we check if any excluded tool appears in the same "window"
	// where this tool appears
	for _, excl := range rule.Excludes {
		excl = strings.TrimSpace(excl)
		if excl == "" {
			continue
		}

		// Find if excluded tool appears in a reasonable window around this occurrence
		exclIndices := toolIndices[excl]
		windowStart := v.findPhaseStart(transcript, occurrence)
		windowEnd := v.findPhaseEnd(transcript, occurrence)

		for _, idx := range exclIndices {
			if idx >= windowStart && idx <= windowEnd && idx != occurrence {
				failures = append(failures, fmt.Sprintf("dependency violation: %s (at index %d) excludes %s but found at index %d in same phase",
					rule.Tool, occurrence, excl, idx))
				break
			}
		}
	}

	return failures
}

// findPhaseStart finds the start of the current "phase" (simplified)
// A phase is defined as a sequence bounded by certain boundary tools
func (v *DependencyValidator) findPhaseStart(transcript *ToolTranscriptArtifact, index int) int {
	// Simplified: look backwards for phase boundary tools
	boundaryTools := map[string]bool{
		"checkpoint":    true,
		"git_commit":    true,
		"memory_commit": true,
	}

	for i := index - 1; i >= 0; i-- {
		if boundaryTools[strings.ToLower(transcript.Entries[i].Tool)] {
			return i + 1
		}
	}
	return 0
}

// findPhaseEnd finds the end of the current "phase" (simplified)
func (v *DependencyValidator) findPhaseEnd(transcript *ToolTranscriptArtifact, index int) int {
	// Simplified: look forwards for phase boundary tools
	boundaryTools := map[string]bool{
		"checkpoint":    true,
		"git_commit":    true,
		"memory_commit": true,
	}

	for i := index + 1; i < len(transcript.Entries); i++ {
		if boundaryTools[strings.ToLower(transcript.Entries[i].Tool)] {
			return i - 1
		}
	}
	return len(transcript.Entries) - 1
}

// ValidateToolOrdering checks if tools appear in the expected order
// Supports "adjacent" modifier for consecutive tool assertions
func ValidateToolOrdering(transcript *ToolTranscriptArtifact, expected []string, adjacent bool) []string {
	var failures []string

	if len(expected) == 0 {
		return nil
	}

	if transcript == nil || len(transcript.Entries) == 0 {
		return []string{"ordering violation: no tool calls in transcript"}
	}

	// Build list of actual tool calls
	actual := make([]string, 0, len(transcript.Entries))
	for _, entry := range transcript.Entries {
		actual = append(actual, strings.TrimSpace(entry.Tool))
	}

	if adjacent {
		// Check for consecutive appearance
		for i := 0; i <= len(actual)-len(expected); i++ {
			match := true
			for j, exp := range expected {
				if actual[i+j] != strings.TrimSpace(exp) {
					match = false
					break
				}
			}
			if match {
				return nil // Found consecutive match
			}
		}
		failures = append(failures, fmt.Sprintf("adjacent ordering violation: expected %v to appear consecutively in %v",
			expected, actual))
	} else {
		// Check for any order appearance (non-consecutive allowed)
		positions := make([]int, len(expected))
		for i, exp := range expected {
			expTrimmed := strings.TrimSpace(exp)
			found := -1
			for j, act := range actual {
				if act == expTrimmed {
					found = j
					break
				}
			}
			if found == -1 {
				failures = append(failures, fmt.Sprintf("ordering violation: expected tool %s not found in transcript", exp))
				continue
			}
			positions[i] = found
		}

		// Check that positions are in increasing order
		for i := 1; i < len(positions); i++ {
			if positions[i] <= positions[i-1] {
				failures = append(failures, fmt.Sprintf("ordering violation: %s (at position %d) must come before %s (at position %d)",
					expected[i-1], positions[i-1], expected[i], positions[i]))
			}
		}
	}

	return failures
}

// HasToolDependency checks if a specific dependency rule exists
func HasToolDependency(dependencies []ToolDependency, tool string, requires string) bool {
	tool = strings.TrimSpace(tool)
	requires = strings.TrimSpace(requires)

	for _, dep := range dependencies {
		if strings.TrimSpace(dep.Tool) == tool {
			for _, req := range dep.Requires {
				if strings.TrimSpace(req) == requires {
					return true
				}
			}
		}
	}
	return false
}

// AddDependency adds a dependency rule to the list if not already present
func AddDependency(dependencies []ToolDependency, newDep ToolDependency) []ToolDependency {
	// Check if this exact dependency already exists
	if HasToolDependency(dependencies, newDep.Tool, newDep.Requires[0]) {
		return dependencies
	}
	return append(dependencies, newDep)
}
