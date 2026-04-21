package runtime

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	euclorelurpic "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
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

func TestPrimaryRelurpicCapabilityForWork_UsesCapabilitySequence(t *testing.T) {
	// The classifier sets CapabilitySequence; primaryRelurpicCapabilityForWork
	// should use it directly without re-scanning keywords or signals.
	tests := []struct {
		name        string
		sequence    []string
		expectedCap string
	}{
		{
			name:        "debug_repair_simple_from_sequence",
			sequence:    []string{euclorelurpic.CapabilityDebugRepairSimple},
			expectedCap: euclorelurpic.CapabilityDebugRepairSimple,
		},
		{
			name:        "debug_investigate_repair_from_sequence",
			sequence:    []string{euclorelurpic.CapabilityDebugInvestigateRepair},
			expectedCap: euclorelurpic.CapabilityDebugInvestigateRepair,
		},
		{
			name:        "multi_element_sequence_uses_first",
			sequence:    []string{euclorelurpic.CapabilityDebugRepairSimple, euclorelurpic.CapabilityDebugFlawSurface},
			expectedCap: euclorelurpic.CapabilityDebugRepairSimple,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := TaskEnvelope{
				Instruction:        "any instruction",
				EditPermitted:      true,
				CapabilitySequence: tt.sequence,
			}
			mode := ModeResolution{ModeID: "debug"}

			capID := primaryRelurpicCapabilityForWork(envelope, mode)

			if capID != tt.expectedCap {
				t.Errorf("primaryRelurpicCapabilityForWork() = %q, want %q",
					capID, tt.expectedCap)
			}
		})
	}
}
