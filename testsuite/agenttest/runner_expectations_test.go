package agenttest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeotensions "github.com/lexcodex/relurpify/archaeo/tensions"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
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

func TestEvaluateEucloExpectationsMatchesAssuranceAndResultState(t *testing.T) {
	snapshot := &core.ContextSnapshot{
		State: map[string]any{
			"euclo.execution_status": map[string]any{
				"result_class":    "failed",
				"assurance_class": "repair_exhausted",
			},
			"euclo.proof_surface": map[string]any{
				"assurance_class":     "repair_exhausted",
				"recovery_status":     "repair_exhausted",
				"success_gate_reason": "verification_failed",
			},
		},
	}

	failures := evaluateEucloExpectations(&EucloExpectSpec{
		ResultClass:       "failed",
		AssuranceClass:    "repair_exhausted",
		SuccessGateReason: "verification_failed",
		RecoveryStatus:    "repair_exhausted",
	}, snapshot)
	if len(failures) > 0 {
		t.Fatalf("expected assurance/result expectations to pass, got %v", failures)
	}
}

func TestEvaluateEucloExpectationsMatchesBehaviorTrace(t *testing.T) {
	snapshot := &core.ContextSnapshot{
		State: map[string]any{
			"euclo.relurpic_behavior_trace": map[string]any{
				"primary_capability_id":      "euclo:chat.implement",
				"supporting_routines":        []any{"euclo:chat.direct-edit-execution", "euclo:chat.targeted-verification-repair"},
				"recipe_ids":                 []any{"chat.implement.edit", "chat.implement.verify"},
				"specialized_capability_ids": []any{"euclo.execution.react"},
				"executor_family":            "react",
			},
		},
	}

	failures := evaluateEucloExpectations(&EucloExpectSpec{
		BehaviorFamily:                 "react",
		PrimaryRelurpicCapability:      "euclo:chat.implement",
		SupportingRelurpicCapabilities: []string{"euclo:chat.direct-edit-execution", "euclo:chat.targeted-verification-repair"},
		SpecializedCapabilityIDs:       []string{"euclo.execution.react"},
		RecipeIDs:                      []string{"chat.implement.edit", "chat.implement.verify"},
	}, snapshot)
	if len(failures) > 0 {
		t.Fatalf("expected behavior trace expectations to pass, got %v", failures)
	}
}

func TestEvaluateEucloExpectationsReportsMissingBehaviorTraceFields(t *testing.T) {
	snapshot := &core.ContextSnapshot{
		State: map[string]any{
			"euclo.relurpic_behavior_trace": map[string]any{
				"primary_capability_id": "euclo:debug.investigate",
				"executor_family":       "react",
			},
		},
	}

	failures := evaluateEucloExpectations(&EucloExpectSpec{
		PrimaryRelurpicCapability:      "euclo:debug.investigate",
		SupportingRelurpicCapabilities: []string{"euclo:debug.root-cause"},
		RecipeIDs:                      []string{"debug.investigate.localize"},
	}, snapshot)
	if len(failures) != 2 {
		t.Fatalf("expected two missing behavior trace failures, got %v", failures)
	}
	if !strings.Contains(strings.Join(failures, "; "), `euclo.supporting_relurpic_capabilities: missing "euclo:debug.root-cause"`) {
		t.Fatalf("expected missing supporting capability failure, got %v", failures)
	}
	if !strings.Contains(strings.Join(failures, "; "), `euclo.recipe_ids: missing "debug.investigate.localize"`) {
		t.Fatalf("expected missing recipe failure, got %v", failures)
	}
}

func TestEvaluateEucloExpectationsMatchesDegradationModeFromSuccessGate(t *testing.T) {
	snapshot := &core.ContextSnapshot{
		State: map[string]any{
			"euclo.success_gate": map[string]any{
				"result_class":     "completed_with_deferrals",
				"assurance_class":  "operator_deferred",
				"degradation_mode": "operator_waiver",
			},
		},
	}

	failures := evaluateEucloExpectations(&EucloExpectSpec{
		ResultClass:     "completed_with_deferrals",
		AssuranceClass:  "operator_deferred",
		DegradationMode: "operator_waiver",
	}, snapshot)
	if len(failures) > 0 {
		t.Fatalf("expected degradation expectations to pass, got %v", failures)
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

func TestContextSnapshotKeyNotEmpty(t *testing.T) {
	snapshot := &core.ContextSnapshot{
		State: map[string]any{
			"plain":   "value",
			"empty":   "",
			"nested":  map[string]any{"items": []any{"x"}},
			"missing": nil,
		},
	}

	if !contextSnapshotKeyNotEmpty(snapshot, "plain") {
		t.Fatal("expected plain key to be non-empty")
	}
	if !contextSnapshotKeyNotEmpty(snapshot, "nested.items") {
		t.Fatal("expected nested items to be non-empty")
	}
	if contextSnapshotKeyNotEmpty(snapshot, "empty") {
		t.Fatal("expected empty string to be treated as empty")
	}
	if contextSnapshotKeyNotEmpty(snapshot, "missing") {
		t.Fatal("expected nil value to be treated as empty")
	}
}

func TestEvaluateExpectationsWorkflowHasTensions(t *testing.T) {
	workspace := t.TempDir()
	paths := config.New(workspace)
	if err := os.MkdirAll(filepath.Dir(paths.WorkflowStateFile()), 0o755); err != nil {
		t.Fatalf("mkdir workflow state dir: %v", err)
	}
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Clean(paths.WorkflowStateFile()))
	if err != nil {
		t.Fatalf("open workflow store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-tensions",
		TaskID:      "task-wf-tensions",
		TaskType:    core.TaskTypeAnalysis,
		Instruction: "seed tensions",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if _, err := (archaeotensions.Service{Store: store}).CreateOrUpdate(context.Background(), archaeotensions.CreateInput{
		WorkflowID:  "wf-tensions",
		Kind:        "semantic_gap",
		Description: "seeded active tension",
		Status:      archaeodomain.TensionUnresolved,
	}); err != nil {
		t.Fatalf("seed tension: %v", err)
	}

	err = evaluateExpectations(ExpectSpec{
		WorkflowHasTensions: []string{"wf-tensions"},
	}, workspace, "", nil, nil, nil, TokenUsageReport{}, MemoryOutcomeReport{}, &core.ContextSnapshot{})
	if err != nil {
		t.Fatalf("expected workflow_has_tensions to pass, got %v", err)
	}
}

func TestEvaluateExpectationsStateKeyNotEmpty(t *testing.T) {
	snapshot := &core.ContextSnapshot{
		State: map[string]any{
			"euclo.active_exploration_id": "exploration-1",
		},
	}

	err := evaluateExpectations(ExpectSpec{
		StateKeysNotEmpty: []string{"euclo.active_exploration_id"},
	}, t.TempDir(), "", nil, nil, nil, TokenUsageReport{}, MemoryOutcomeReport{}, snapshot)
	if err != nil {
		t.Fatalf("expected state_key_not_empty to pass, got %v", err)
	}
}
