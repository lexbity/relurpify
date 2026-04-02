package modes

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

type scriptedEmitter struct {
	interaction.NoopEmitter
	responses []interaction.UserResponse
}

func (e *scriptedEmitter) AwaitResponse(ctx context.Context) (interaction.UserResponse, error) {
	if len(e.responses) == 0 {
		return e.NoopEmitter.AwaitResponse(ctx)
	}
	resp := e.responses[0]
	e.responses = e.responses[1:]
	return resp, ctx.Err()
}

func TestTestResultPhase_UsesRedEvidenceFromState(t *testing.T) {
	emitter := &scriptedEmitter{responses: []interaction.UserResponse{{ActionID: "implement"}}}
	phase := &TestResultPhase{}
	outcome, err := phase.Execute(context.Background(), interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"euclo.tdd.red_evidence": map[string]any{
				"status":  "fail",
				"summary": "targeted tests failed as expected",
				"checks": []any{
					map[string]any{"name": "go test ./pkg/auth", "details": "--- FAIL: TestLoginRegression", "working_directory": "/workspace"},
				},
			},
		},
		Mode:       "tdd",
		Phase:      "review_tests",
		PhaseIndex: 2,
		PhaseCount: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !outcome.Advance {
		t.Fatal("expected phase to advance")
	}
	frame := emitter.Frames[len(emitter.Frames)-1]
	result, ok := frame.Content.(interaction.ResultContent)
	if !ok {
		t.Fatalf("expected result content, got %#v", frame.Content)
	}
	if result.Status != "all_red" {
		t.Fatalf("expected all_red result, got %#v", result)
	}
	if len(result.Evidence) == 0 || result.Evidence[0].Detail == "" {
		t.Fatalf("expected evidence details, got %#v", result)
	}
}

func TestGreenStatusPhase_RefactorStaysInTDDFlow(t *testing.T) {
	emitter := &scriptedEmitter{responses: []interaction.UserResponse{{ActionID: "refactor"}}}
	phase := &GreenStatusPhase{}
	outcome, err := phase.Execute(context.Background(), interaction.PhaseMachineContext{
		Emitter: emitter,
		State: map[string]any{
			"euclo.tdd.green_evidence": map[string]any{
				"status":  "pass",
				"summary": "all tests passed",
			},
		},
		Mode:       "tdd",
		Phase:      "green",
		PhaseIndex: 4,
		PhaseCount: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.JumpTo != "implement" {
		t.Fatalf("expected refactor to jump back to implement, got %#v", outcome)
	}
	if requested, _ := outcome.StateUpdates["tdd.refactor_requested"].(bool); !requested {
		t.Fatalf("expected refactor request state update, got %#v", outcome.StateUpdates)
	}
}

func TestBuildGreenActions_HidesRefactorAfterRequest(t *testing.T) {
	actions := buildGreenActions(map[string]any{"tdd.refactor_requested": true}, interaction.ResultContent{Status: "passed"})
	for _, action := range actions {
		if action.ID == "refactor" {
			t.Fatalf("did not expect refactor action once already requested: %#v", actions)
		}
	}
}
