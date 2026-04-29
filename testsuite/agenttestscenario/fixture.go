package agenttestscenario

import (
	"testing"

	chaintelemetry "codeburg.org/lexbit/relurpify/agents/chainer/telemetry"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
)

type Fixture struct {
	T         testing.TB
	Env       agentenv.WorkspaceEnvironment
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
	Events []core.Event
}

func (t *TelemetryRecorder) Emit(event core.Event) {
	t.Events = append(t.Events, event)
}

// NewFixture wires the shared scenario model, a no-op executor, telemetry,
// and an echo tool so agent scenarios can focus on behavior.
func NewFixture(t testing.TB, turns ...ScenarioModelTurn) *Fixture {
	t.Helper()

	env := agentenv.WorkspaceEnvironment{
		Config: &core.Config{Name: "test", Model: "stub", MaxIterations: 1},
	}
	model := &ScenarioStubModel{}
	telemetry := &TelemetryRecorder{}
	return newFixture(t, env, model, telemetry)
}

func newFixture(t testing.TB, env agentenv.WorkspaceEnvironment, model *ScenarioStubModel, telemetry *TelemetryRecorder) *Fixture {
	t.Helper()

	if env.Config == nil {
		env.Config = &core.Config{Name: "test", Model: "stub", MaxIterations: 1}
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
