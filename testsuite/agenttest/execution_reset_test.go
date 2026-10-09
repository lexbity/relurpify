package agenttest

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/app/envcomposition"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution"
)

// Fixture strings for the AC-13 descriptor. They are constants because the
// package already repeats these values widely (goconst).
const (
	resetTestSuitePath   = "/suite.yaml"
	resetTestSuiteName   = "suite"
	resetTestCaseName    = "case"
	resetTestAgentName   = "agent"
	resetTestInstruction = "do it"
	resetTestBoom        = "boom"
)

// resetDescriptor builds a normalized descriptor carrying the trigger patterns.
// Fields required by Validate are populated with throwaway absolute paths; the
// patterns are compiled by Normalize (the prepare path), not set by hand.
func resetDescriptor(t *testing.T, patterns []string, strategy string, maxRetries int) *PreparedRunDescriptor {
	t.Helper()
	desc := &PreparedRunDescriptor{
		RunID:                "run-reset",
		SuitePath:            resetTestSuitePath,
		SuiteName:            resetTestSuiteName,
		CaseName:             resetTestCaseName,
		AgentName:            resetTestAgentName,
		Instruction:          resetTestInstruction,
		WorkspaceRoot:        "/workspace",
		RunRoot:              "/workspace/run",
		DerivedWorkspaceRoot: "/workspace/run/setup/workspace",
		ConfigPath:           "/workspace/run/setup/workspace/relurpify_cfg/config.yaml",
		SetupDir:             "/workspace/run/setup",
		ExecutionDir:         "/workspace/run/execution",
		BackendSelection:     PreparedRunSelectionSingle,
		BackendProvider:      ollama,
		BackendFamily:        ollama,
		BackendEndpoint:      "http://localhost:11434",
		MaxRetries:           maxRetries,
		BackendResetStrategy: strategy,
		BackendResetOn:       patterns,
	}
	require.NoError(t, desc.Normalize())
	return desc
}

// runResetRetry executes executeWithRetry against a failing stub and a counting
// backend, returning the observable retry surface (error last per revive).
func runResetRetry(t *testing.T, desc *PreparedRunDescriptor, failCount int, failWith error) (*execution.Result, int, []string, *fakeManagedBackend, error) {
	t.Helper()
	exec := &PreparedRunExecutor{}
	exec.WithAgentOverride(&fakeAgentExecutor{failCount: failCount, failWith: failWith})
	backend := &fakeManagedBackend{}
	exec.model = &envcomposition.ModelRuntime{Backend: backend}

	task := &execution.Task{ID: desc.RunID, Instruction: desc.Instruction}
	env := contextdata.NewEnvelope(desc.RunID, desc.RunID)
	result, attempts, triggeredBy, err := exec.executeWithRetry(context.Background(), desc, task, env, io.Discard)
	return result, attempts, triggeredBy, backend, err
}

// TestResetPatternInvalidFailsPrepare proves an unparseable pattern fails at
// preparation with the offending pattern named (FR-16).
func TestResetPatternInvalidFailsPrepare(t *testing.T) {
	desc := &PreparedRunDescriptor{BackendResetOn: []string{"[invalid"}}
	err := desc.Normalize()
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid backend-reset-on pattern")
	require.Contains(t, err.Error(), "[invalid")
}

// TestExecuteWithRetryResetOn is the AC-13 headline: a matching error fires a
// reset and a retry, is recorded in triggeredBy, and defaults the reset
// strategy to model when none was declared.
func TestExecuteWithRetryResetOn(t *testing.T) {
	desc := resetDescriptor(t, []string{"(?i)connection reset"}, "", 2)

	result, attempts, triggeredBy, backend, err := runResetRetry(t, desc, 1, fmt.Errorf("connection reset by peer"))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Equal(t, 2, attempts, "one retry after the matched failure")
	require.Equal(t, []string{"connection reset by peer"}, triggeredBy)
	require.Equal(t, 1, backend.resetCount, "an unset strategy defaults to a model reset when patterns are set")
}

// TestResetPatternNoMatchDoesNotRetry proves a non-matching error neither
// spends the retry budget nor fires a reset.
func TestResetPatternNoMatchDoesNotRetry(t *testing.T) {
	desc := resetDescriptor(t, []string{"(?i)connection reset"}, "", 3)

	result, attempts, triggeredBy, backend, err := runResetRetry(t, desc, 5, fmt.Errorf("model unavailable"))
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, 1, attempts, "a non-matching error must not spend the retry budget")
	require.Empty(t, triggeredBy)
	require.Equal(t, 0, backend.resetCount)
}

// TestResetPatternExplicitStrategyRespected proves an explicit model strategy
// resets once per matched retry and stops at the retry bound.
func TestResetPatternExplicitStrategyRespected(t *testing.T) {
	desc := resetDescriptor(t, []string{resetTestBoom}, resetStrategyModel, 2)

	result, attempts, triggeredBy, backend, err := runResetRetry(t, desc, 5, fmt.Errorf("boom"))
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, 3, attempts, "MaxRetries=2 allows one initial attempt plus two retries")
	require.Len(t, triggeredBy, 2)
	require.Equal(t, 2, backend.resetCount)
}

// TestResetPatternsRoundTripThroughDescriptor proves the patterns survive the
// persisted descriptor handoff (the executor loads them from disk).
func TestResetPatternsRoundTripThroughDescriptor(t *testing.T) {
	desc := resetDescriptor(t, []string{resetTestBoom, "reset"}, resetStrategyModel, 1)

	path := filepath.Join(t.TempDir(), "descriptor.json")
	require.NoError(t, desc.Write(path))

	loaded, err := LoadPreparedRunDescriptor(path)
	require.NoError(t, err)
	require.Equal(t, []string{resetTestBoom, "reset"}, loaded.BackendResetOn)
	require.Len(t, loaded.resetPatterns, 2, "loading must recompile the patterns")
}
