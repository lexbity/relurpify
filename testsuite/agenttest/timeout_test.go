package agenttest

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/app/envcomposition"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	llm "codeburg.org/lexbit/relurpify/platform/llm"
)

// slowExecutor sleeps past any timeout before returning success.
type slowExecutor struct {
	sleep time.Duration
}

func (x *slowExecutor) Initialize(config *execution.Config) error { return nil }
func (x *slowExecutor) Capabilities() []string                    { return nil }
func (x *slowExecutor) BuildGraph(context.Context, *execution.Task) (*agentgraph.Graph, error) {
	return nil, errors.New("not used")
}
func (x *slowExecutor) Execute(ctx context.Context, _ *execution.Task, _ *contextdata.Envelope) (*execution.Result, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(x.sleep):
	}
	return &execution.Result{NodeID: "root", Success: true}, nil
}

func TestEffectiveTimeoutPrecedence(t *testing.T) {
	cases := []struct {
		name   string
		c      string
		suite  string
		cli    time.Duration
		cliSet bool
		want   time.Duration
	}{
		{"default_when_nothing_set", "", "", 0, false, DefaultCaseTimeout},
		{"case_wins", "2s", "5m", time.Minute, true, 2 * time.Second},
		{"suite_when_no_case", "", "5m", time.Minute, true, 5 * time.Minute},
		{"cli_when_nothing_else", "", "", 30 * time.Second, true, 30 * time.Second},
		{"literal_zero_case_disables", "0", "5m", time.Minute, true, 0},
		{"literal_zero_suite_disables", "", "0", time.Minute, true, 0},
		{"literal_zero_cli_disables", "", "", 0, true, 0},
		{"zero_cli_without_flag_is_default", "", "", 0, false, DefaultCaseTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, EffectiveTimeout(tc.c, tc.suite, tc.cli, tc.cliSet))
		})
	}
}

// TestCaseTimeoutAborts proves the bound: a case whose executor sleeps past
// the effective timeout fails with the typed ErrCaseTimeout (infra class
// "timeout", classified by the typed path) within the budget plus slack.
func TestCaseTimeoutAborts(t *testing.T) {
	e := &PreparedRunExecutor{}
	e.agentOverride = &slowExecutor{sleep: 10 * time.Second}
	desc := &PreparedRunDescriptor{
		RunID:            "run-timeout",
		Instruction:      "sleep",
		EffectiveTimeout: 300 * time.Millisecond,
		MaxRetries:       0,
	}
	started := time.Now()
	result, attempts, _, err := e.executeWithRetry(context.Background(), desc, &execution.Task{ID: "run-timeout"}, contextdata.NewEnvelope("run-timeout", "run-timeout"), nil)
	_ = result
	elapsed := time.Since(started)
	require.Equal(t, 1, attempts)
	require.Error(t, err)
	var timeout *ErrCaseTimeout
	require.ErrorAs(t, err, &timeout)
	require.Equal(t, 300*time.Millisecond, timeout.After)
	require.Less(t, elapsed, 5*time.Second, "the case aborts at the budget, not after the sleep")

	report := e.buildCaseReport(desc, nil, err, 1, nil, nil, time.Now().UTC())
	require.False(t, report.Success)
	require.Equal(t, "timeout", report.FailureKind)
	require.Equal(t, "typed", report.ClassifiedBy)
}

// TestTimeoutZeroDisables proves the explicit opt-out: an effective timeout
// of zero runs unbounded (the slow executor completes).
func TestTimeoutZeroDisables(t *testing.T) {
	e := &PreparedRunExecutor{}
	e.agentOverride = &slowExecutor{sleep: 50 * time.Millisecond}
	desc := &PreparedRunDescriptor{
		RunID:            "run-unbounded",
		Instruction:      "sleep a little",
		EffectiveTimeout: 0,
		MaxRetries:       0,
	}
	_, _, _, err := e.executeWithRetry(context.Background(), desc, &execution.Task{ID: "run-unbounded"}, contextdata.NewEnvelope("run-unbounded", "run-unbounded"), nil)
	require.NoError(t, err)
}

// TestTimeoutRetriesWithFreshBudget proves a timeout consumes the reset-on
// patterns like any retryable infra failure: the retry gets a fresh attempt
// budget.
// resetCountingBackend records Reset calls; the timeout retry runs the
// pattern-gated reset path.
type resetCountingBackend struct{ resets int }

func (b *resetCountingBackend) Model() llm.LanguageModel { return nil }
func (b *resetCountingBackend) Embedder() llm.Embedder   { return nil }
func (b *resetCountingBackend) Capabilities() llm.BackendCapabilities {
	return llm.BackendCapabilities{}
}
func (b *resetCountingBackend) ModelContextSize(context.Context) (int, error) {
	return 0, nil
}
func (b *resetCountingBackend) Health(context.Context) (*llm.HealthReport, error) {
	return &llm.HealthReport{}, nil
}
func (b *resetCountingBackend) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return nil, nil
}
func (b *resetCountingBackend) Warm(context.Context) error   { return nil }
func (b *resetCountingBackend) Close() error                 { return nil }
func (b *resetCountingBackend) SetDebugLogging(bool)         {}
func (b *resetCountingBackend) SetProfile(*llm.ModelProfile) {}
func (b *resetCountingBackend) Reset(context.Context, string) error {
	b.resets++
	return nil
}

func TestTimeoutRetriesWithFreshBudget(t *testing.T) {
	e := &PreparedRunExecutor{}
	// First attempt sleeps past the (tiny) budget; the retry succeeds fast.
	x := &sequencedSlowExecutor{sleeps: []time.Duration{time.Second, 0}}
	e.agentOverride = x
	e.model = &envcomposition.ModelRuntime{Backend: &resetCountingBackend{}}
	desc := &PreparedRunDescriptor{
		RunID:            "run-timeout-retry",
		Instruction:      "sleep then succeed",
		EffectiveTimeout: 100 * time.Millisecond,
		MaxRetries:       1,
		resetPatterns:    mustPatterns(t, `(?i)case timeout`),
	}
	_, attempts, _, err := e.executeWithRetry(context.Background(), desc, &execution.Task{ID: "run-timeout-retry"}, contextdata.NewEnvelope("run-timeout-retry", "run-timeout-retry"), nil)
	require.NoError(t, err, "the retry (fresh budget) succeeds")
	require.Equal(t, 2, attempts)
	require.Equal(t, 2, x.calls)
	require.Equal(t, 1, e.model.Backend.(*resetCountingBackend).resets,
		"the pattern-gated reset ran between the attempts")
}

func mustPatterns(t *testing.T, raw ...string) []*regexp.Regexp {
	t.Helper()
	patterns, err := compileResetPatterns(raw)
	require.NoError(t, err)
	return patterns
}

// sequencedSlowExecutor sleeps sleeps[i] on call i, then returns success.
type sequencedSlowExecutor struct {
	sleeps []time.Duration
	calls  int
}

func (x *sequencedSlowExecutor) Initialize(config *execution.Config) error { return nil }
func (x *sequencedSlowExecutor) Capabilities() []string                    { return nil }
func (x *sequencedSlowExecutor) BuildGraph(context.Context, *execution.Task) (*agentgraph.Graph, error) {
	return nil, errors.New("not used")
}

func (x *sequencedSlowExecutor) Execute(ctx context.Context, _ *execution.Task, _ *contextdata.Envelope) (*execution.Result, error) {
	i := x.calls
	x.calls++
	if i < len(x.sleeps) && x.sleeps[i] > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(x.sleeps[i]):
		}
	}
	return &execution.Result{NodeID: "root", Success: true}, nil
}
