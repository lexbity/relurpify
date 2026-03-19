package agenttest

import (
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestEvaluateEucloExpectationsRequiresRecoveryTrace(t *testing.T) {
	snapshot := &core.ContextSnapshot{
		State: map[string]any{
			"euclo.interaction_state": map[string]any{"mode": "code"},
		},
	}

	failures := evaluateEucloExpectations(&EucloExpectSpec{
		Mode:              "code",
		RecoveryAttempted: true,
	}, snapshot)

	if len(failures) != 1 || !strings.Contains(failures[0], "euclo.recovery_trace is nil") {
		t.Fatalf("expected missing recovery trace failure, got %v", failures)
	}
}

func TestEvaluateEucloExpectationsMatchesRecoveryStrategies(t *testing.T) {
	snapshot := &core.ContextSnapshot{
		State: map[string]any{
			"euclo.interaction_state": map[string]any{"mode": "debug"},
			"euclo.recovery_trace": map[string]any{
				"attempts": []any{
					map[string]any{"level": "capability", "strategy": "capability_fallback", "from": "a", "to": "b", "success": true},
					map[string]any{"level": "paradigm", "strategy": "paradigm_switch", "from": "react", "to": "pipeline", "success": true},
				},
			},
		},
	}

	failures := evaluateEucloExpectations(&EucloExpectSpec{
		Mode:               "debug",
		RecoveryAttempted:  true,
		RecoveryStrategies: []string{"capability_fallback", "paradigm_switch"},
	}, snapshot)
	if len(failures) > 0 {
		t.Fatalf("expected recovery expectations to pass, got %v", failures)
	}
}

func TestEvaluateEucloExpectationsReportsMissingRecoveryStrategy(t *testing.T) {
	snapshot := &core.ContextSnapshot{
		State: map[string]any{
			"euclo.recovery_trace": map[string]any{
				"attempts": []any{
					map[string]any{"level": "capability", "strategy": "capability_fallback", "success": true},
				},
			},
		},
	}

	failures := evaluateEucloExpectations(&EucloExpectSpec{
		RecoveryStrategies: []string{"paradigm_switch"},
	}, snapshot)
	if len(failures) != 1 || !strings.Contains(failures[0], `expected recovery strategy "paradigm_switch"`) {
		t.Fatalf("expected missing strategy failure, got %v", failures)
	}
}

func TestEvaluateEucloExpectationsMatchesPhasesExecuted(t *testing.T) {
	snapshot := &core.ContextSnapshot{
		State: map[string]any{
			"euclo.interaction_state": map[string]any{
				"mode":            "code",
				"phases_executed": []any{"understand", "scope", "generate", "commit", "execute"},
			},
		},
	}

	failures := evaluateEucloExpectations(&EucloExpectSpec{
		Mode:           "code",
		PhasesExecuted: []string{"scope", "generate", "execute"},
	}, snapshot)
	if len(failures) > 0 {
		t.Fatalf("expected phase execution expectations to pass, got %v", failures)
	}
}

func TestEvaluateEucloExpectationsMatchesArtifactChain(t *testing.T) {
	snapshot := &core.ContextSnapshot{
		State: map[string]any{
			"euclo.interaction_records": []any{
				map[string]any{
					"phase":              "commit",
					"artifacts_produced": []any{"euclo.plan"},
					"produced_artifacts": []any{
						map[string]any{
							"kind":    "euclo.plan",
							"summary": "rate limit plan",
							"payload": map[string]any{"steps": []any{"add rate limiting"}},
						},
					},
				},
				map[string]any{
					"phase":              "execute",
					"artifacts_consumed": []any{"euclo.plan"},
				},
			},
		},
	}

	failures := evaluateEucloExpectations(&EucloExpectSpec{
		ArtifactChain: []ArtifactChainSpec{{
			Kind:            "plan",
			ProducedByPhase: "commit",
			ConsumedByPhase: "execute",
			ContentContains: []string{"rate"},
		}},
	}, snapshot)
	if len(failures) > 0 {
		t.Fatalf("expected artifact chain expectations to pass, got %v", failures)
	}
}

func TestEvaluateEucloExpectationsReportsMissingArtifactChainContent(t *testing.T) {
	snapshot := &core.ContextSnapshot{
		State: map[string]any{
			"euclo.interaction_records": []any{
				map[string]any{
					"phase":              "execute",
					"artifacts_produced": []any{"euclo.edit_intent"},
					"artifacts_consumed": []any{"euclo.plan"},
					"produced_artifacts": []any{
						map[string]any{"kind": "euclo.edit_intent", "summary": "small fix"},
					},
				},
			},
		},
	}

	failures := evaluateEucloExpectations(&EucloExpectSpec{
		ArtifactChain: []ArtifactChainSpec{{
			Kind:            "edit_intent",
			ProducedByPhase: "execute",
			ConsumedByPhase: "verify",
			ContentContains: []string{"validation"},
		}},
	}, snapshot)
	if len(failures) == 0 {
		t.Fatal("expected artifact chain failure")
	}
	if !strings.Contains(strings.Join(failures, "; "), `missing "validation"`) {
		t.Fatalf("expected missing content failure, got %v", failures)
	}
}
