package agenttestscenario

import (
	"testing"

	chaintelemetry "codeburg.org/lexbit/relurpify/cognitionzoo/chainer/telemetry"
	execution "codeburg.org/lexbit/relurpify/execution"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

type Fixture struct {
	T         testing.TB
	Config    *execution.Config
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

	cfg := &execution.Config{Name: "test", Model: "stub", MaxIterations: 1}
	model := &ScenarioStubModel{}
	telemetry := &TelemetryRecorder{}
	return newFixture(t, cfg, model, telemetry)
}

func newFixture(t testing.TB, cfg *execution.Config, model *ScenarioStubModel, telemetry *TelemetryRecorder) *Fixture {
	t.Helper()

	if cfg == nil {
		cfg = &execution.Config{Name: "test", Model: "stub", MaxIterations: 1}
	}
	cfg.Telemetry = telemetry
	return &Fixture{
		T:         t,
		Config:    cfg,
		Model:     model,
		Exec:      &NoopExecutor{},
		Telemetry: telemetry,
		Events:    chaintelemetry.NewEventRecorder(),
	}
}
