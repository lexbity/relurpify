package agenttestscenario

import (
	"testing"

	chaintelemetry "codeburg.org/lexbit/relurpify/agents/chainer/telemetry"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/core"
	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
)

type Fixture struct {
	T         testing.TB
	Env       agentenv.AgentEnvironment
	Model     *testutil.ScenarioStubModel
	Exec      *testutil.NoopExecutor
	Telemetry *testutil.TelemetryRecorder
	Events    *chaintelemetry.EventRecorder
}

// NewFixture wires the shared scenario model, a no-op executor, telemetry,
// and an echo tool so agent scenarios can focus on behavior.
func NewFixture(t testing.TB, turns ...testutil.ScenarioModelTurn) *Fixture {
	t.Helper()

	env, model := testutil.EnvWithScenarioModel(t, turns...)
	env.Registry = testutil.RegistryWith(testutil.EchoTool{})

	telemetry := &testutil.TelemetryRecorder{}
	return newFixture(t, env, model, telemetry)
}

func newFixture(t testing.TB, env agentenv.AgentEnvironment, model *testutil.ScenarioStubModel, telemetry *testutil.TelemetryRecorder) *Fixture {
	t.Helper()

	if env.Config == nil {
		env.Config = &core.Config{Name: "test", Model: "stub", MaxIterations: 1}
	}
	env.Config.Telemetry = telemetry
	return &Fixture{
		T:         t,
		Env:       env,
		Model:     model,
		Exec:      &testutil.NoopExecutor{},
		Telemetry: telemetry,
		Events:    chaintelemetry.NewEventRecorder(),
	}
}
