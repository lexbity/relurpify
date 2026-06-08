package agenttestscenario

import (
	"testing"

	chaintelemetry "codeburg.org/lexbit/relurpify/agentschainer/telemetry"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentenv"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

type Fixture struct {
	T         testing.TB
	Env       agentenv.AgentContext
	Model     *ScenarioStubModel
	Exec      *NoopExecutor
	Telemetry *TelemetryRecorder
	Events    *chaintelemetry.EventRecorder
}

type ScenarioModelTurn struct{}

type ScenarioStubModel struct{}

func (m *ScenarioStubModel) AssertExhausted(tb testing.TB) {}

type NoopExecutor struct {
	Calls int
}

type TelemetryRecorder struct {
	Events []telemetry.Event
}

func (t *TelemetryRecorder) Emit(event telemetry.Event) {
	t.Events = append(t.Events, event)
}

// NewFixture wires the shared scenario model, a no-op executor, telemetry,
// and an echo tool so agent scenarios can focus on behavior.
func NewFixture(t testing.TB, turns ...ScenarioModelTurn) *Fixture {
	t.Helper()

	env := agentenv.AgentContext{
		Config: &execution.Config{Name: "test", Model: "stub", MaxIterations: 1},
	}
	model := &ScenarioStubModel{}
	telemetry := &TelemetryRecorder{}
	return newFixture(t, env, model, telemetry)
}

func newFixture(t testing.TB, env agentenv.AgentContext, model *ScenarioStubModel, telemetry *TelemetryRecorder) *Fixture {
	t.Helper()

	if env.Config == nil {
		env.Config = &execution.Config{Name: "test", Model: "stub", MaxIterations: 1}
	}
	env.Config.Telemetry = telemetry
	return &Fixture{
		T:         t,
		Env:       env,
		Model:     model,
		Exec:      &NoopExecutor{},
		Telemetry: telemetry,
		Events:    chaintelemetry.NewEventRecorder(),
	}
}
