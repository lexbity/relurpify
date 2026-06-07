//go:build testonly

package sandbox

import (
	"context"
	"fmt"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// FakeRunner is a programmable CommandRunner for tests.
// It returns canned results configured via AddResult or the RunFunc hook.
type FakeRunner struct {
	mu      sync.Mutex
	results []ports.CommandResult
	cursor  int
	RunFunc func(ctx context.Context, req CommandRequest) (*ports.CommandResult, error)
}

// NewFakeRunner returns a runner that returns the given results in sequence.
func NewFakeRunner(results ...ports.CommandResult) *FakeRunner {
	return &FakeRunner{
		results: append([]ports.CommandResult(nil), results...),
	}
}

// Run implements CommandRunner.
func (f *FakeRunner) Run(ctx context.Context, req CommandRequest) (*ports.CommandResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.RunFunc != nil {
		return f.RunFunc(ctx, req)
	}
	if f.cursor >= len(f.results) {
		return nil, fmt.Errorf("fake runner: no more results (cursor %d)", f.cursor)
	}
	res := f.results[f.cursor]
	f.cursor++
	if res.Duration > 0 {
		time.Sleep(res.Duration)
	}
	if err := exitErrorFromResult(&res); err != nil {
		return &res, err
	}
	return &res, nil
}

// AddResult appends a result to the programmed sequence.
func (f *FakeRunner) AddResult(res ports.CommandResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results = append(f.results, res)
}

// Reset clears all programmed results and the cursor.
func (f *FakeRunner) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results = nil
	f.cursor = 0
	f.RunFunc = nil
}

// exitErrorFromResult returns an error if the result indicates failure.
func exitErrorFromResult(res *ports.CommandResult) error {
	if res == nil {
		return nil
	}
	if res.TornDown || res.TimedOut || res.Signaled || res.ExitCode != 0 {
		return &CommandResultError{Result: *res}
	}
	return nil
}

// CommandResultError wraps a CommandResult as an error for test assertions.
type CommandResultError struct {
	Result ports.CommandResult
}

func (e *CommandResultError) Error() string {
	return fmt.Sprintf("command failed: exit code %d (signaled=%v, timedOut=%v, tornDown=%v, oom=%v)",
		e.Result.ExitCode, e.Result.Signaled, e.Result.TimedOut, e.Result.TornDown, e.Result.OOMKilled)
}

// Compile-time checks.
var _ CommandRunner = (*FakeRunner)(nil)
