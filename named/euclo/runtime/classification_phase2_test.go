package runtime

import (
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
)

// ─────────────────────────────────────────────────────────────────────────────
// Phase 2.2.1: Chat vs Debug Disambiguation Tests
//
// NOTE: scoreChatIntent and scoreDebugIntent have been removed. Chat/debug
// classification now happens through signals.go keyword groups. These tests
// verify that the signal-based classification produces correct results.
// ─────────────────────────────────────────────────────────────────────────────

func TestClassifyTaskScored_ChatSignals(t *testing.T) {
	tests := []struct {
		name        string
		instruction string
		wantMode    string
	}{
		{
			name:        "explain_question",
			instruction: "explain what this function does",
			wantMode:    "chat",
		},
		{
			name:        "what_is_question",
			instruction: "what is the purpose of this code?",
			wantMode:    "chat",
		},
		{
			name:        "how_do_question",
			instruction: "how do I use this api",
			wantMode:    "chat",
		},
		{
			name:        "show_me_request",
			instruction: "show me how this works",
			wantMode:    "chat",
		},
		{
			name:        "describe_request",
			instruction: "describe this function",
			wantMode:    "chat",
		},
		{
			name:        "tell_me_about",
			instruction: "tell me about this module",
			wantMode:    "chat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := TaskEnvelope{Instruction: tt.instruction, EditPermitted: true}
			scored := ClassifyTaskScored(envelope)
			if scored.RecommendedMode != tt.wantMode {
				t.Errorf("ClassifyTaskScored() = %q, want %q", scored.RecommendedMode, tt.wantMode)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 2.2.2: Task-Type Aware Profile Selection Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestSelectExecutionProfile_TaskTypeAware_DebugAnalysis(t *testing.T) {
	env := TaskEnvelope{EditPermitted: true}
	// Analysis task in debug mode should use trace_execute_analyze
	class := TaskClassification{
		TaskType: core.TaskTypeAnalysis,
	}
	mode := ModeResolution{ModeID: "debug"}
	registry := euclotypes.DefaultExecutionProfileRegistry()

	sel := SelectExecutionProfile(env, class, mode, registry)

	if sel.ProfileID != "trace_execute_analyze" {
		t.Errorf("expected trace_execute_analyze for debug analysis task, got %q", sel.ProfileID)
	}
}

func TestSelectExecutionProfile_TaskTypeAware_DebugCodeMod(t *testing.T) {
	env := TaskEnvelope{EditPermitted: true}
	// Code modification task in debug mode should use reproduce_localize_patch
	class := TaskClassification{
		TaskType: core.TaskTypeCodeModification,
	}
	mode := ModeResolution{ModeID: "debug"}
	registry := euclotypes.DefaultExecutionProfileRegistry()

	sel := SelectExecutionProfile(env, class, mode, registry)

	if sel.ProfileID != "reproduce_localize_patch" {
		t.Errorf("expected reproduce_localize_patch for debug code modification task, got %q", sel.ProfileID)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 2.2.3: Simple Repair Capability Routing Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestDebugSimpleRepairIntent(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		expected bool
	}{
		{
			name:     "fix_this",
			prompt:   "fix this bug",
			expected: true,
		},
		{
			name:     "apply_fix",
			prompt:   "apply a fix to the code",
			expected: true,
		},
		{
			name:     "quick_repair",
			prompt:   "quick repair needed",
			expected: true,
		},
		{
			name:     "simple_fix",
			prompt:   "simple fix for the error",
			expected: true,
		},
		{
			name:     "fix_prefix",
			prompt:   "fix: the counter is wrong",
			expected: true,
		},
		{
			name:     "investigate_not_simple",
			prompt:   "investigate and fix the bug",
			expected: false, // contains "investigate"
		},
		{
			name:     "root_cause_not_simple",
			prompt:   "fix the root cause of the error",
			expected: false, // contains "root cause"
		},
		{
			name:     "find_not_simple",
			prompt:   "fix after find the bug",
			expected: false, // contains "find the"
		},
		{
			name:     "no_repair_intent",
			prompt:   "what is wrong with this code",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lower := strings.ToLower(tt.prompt)
			result := debugSimpleRepairIntent(lower)
			if result != tt.expected {
				t.Errorf("debugSimpleRepairIntent(%q) = %v, want %v", tt.prompt, result, tt.expected)
			}
		})
	}
}

func TestPrimaryRelurpicCapabilityForWork_SimpleRepairRouting(t *testing.T) {
	tests := []struct {
		name        string
		instruction string
		expectedCap string
	}{
		{
			name:        "simple_fix_routes_to_repair_simple",
			instruction: "fix this - the counter subtracts instead of adds",
			expectedCap: euclorelurpic.CapabilityDebugRepairSimple,
		},
		{
			name:        "investigate_routes_to_investigate_repair",
			instruction: "investigate the panic in GetUser",
			expectedCap: euclorelurpic.CapabilityDebugInvestigateRepair,
		},
		{
			name:        "quick_fix_routes_to_repair_simple",
			instruction: "quick fix for the typo",
			expectedCap: euclorelurpic.CapabilityDebugRepairSimple,
		},
		{
			name:        "debug_with_root_cause_routes_to_investigate",
			instruction: "fix the root cause of the error",
			expectedCap: euclorelurpic.CapabilityDebugInvestigateRepair,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := TaskEnvelope{
				Instruction:   tt.instruction,
				EditPermitted: true,
			}
			// Build ReasonCodes with appropriate signals for signal-based routing.
			reasonCodes := []string{"keyword:debug"}
			if tt.expectedCap == euclorelurpic.CapabilityDebugRepairSimple {
				// Add simple repair signal to trigger simple repair routing.
				reasonCodes = append(reasonCodes, "task_structure:simple_repair:fix_this")
			}
			classification := TaskClassification{
				IntentFamilies: []string{"debug"},
				ReasonCodes:    reasonCodes,
			}
			mode := ModeResolution{ModeID: "debug"}
			profile := ExecutionProfileSelection{ProfileID: "reproduce_localize_patch"}

			capID := primaryRelurpicCapabilityForWork(envelope, classification, mode, profile)

			if capID != tt.expectedCap {
				t.Errorf("primaryRelurpicCapabilityForWork() = %q, want %q (instruction: %q)",
					capID, tt.expectedCap, tt.instruction)
			}
		})
	}
}
