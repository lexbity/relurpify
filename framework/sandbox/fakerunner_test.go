//go:build testonly

package sandbox

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

func TestFakeRunnerReturnsProgrammedResults(t *testing.T) {
	fr := NewFakeRunner(
		contracts.CommandResult{Stdout: "first", ExitCode: 0},
		contracts.CommandResult{Stdout: "second", Stderr: "warn", ExitCode: 1},
		contracts.CommandResult{Stdout: "third", ExitCode: 0, Signaled: true},
	)

	// First call
	res, err := fr.Run(context.Background(), CommandRequest{Args: []string{"echo", "first"}})
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if res.Stdout != "first" {
		t.Errorf("stdout = %q, want %q", res.Stdout, "first")
	}
	if res.Stderr != "" {
		t.Errorf("stderr = %q, want empty", res.Stderr)
	}

	// Second call — non-zero exit
	res, err = fr.Run(context.Background(), CommandRequest{Args: []string{"false"}})
	if err == nil {
		t.Fatal("second call: expected error for exit code 1")
	}
	var cmdErr *CommandResultError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("expected *CommandResultError, got %T: %v", err, err)
	}
	if cmdErr.Result.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", cmdErr.Result.ExitCode)
	}
	if res.Stdout != "second" {
		t.Errorf("stdout = %q, want %q", res.Stdout, "second")
	}
	if res.Stderr != "warn" {
		t.Errorf("stderr = %q, want %q", res.Stderr, "warn")
	}

	// Third call — signaled
	res, err = fr.Run(context.Background(), CommandRequest{Args: []string{"sleep", "999"}})
	if err == nil {
		t.Fatal("third call: expected error for signaled process")
	}
	if !errors.As(err, &cmdErr) {
		t.Fatalf("expected *CommandResultError, got %T: %v", err, err)
	}
	if !cmdErr.Result.Signaled {
		t.Error("expected signaled flag")
	}
	if res.Stdout != "third" {
		t.Errorf("stdout = %q, want %q", res.Stdout, "third")
	}
}

func TestFakeRunnerOutOfResults(t *testing.T) {
	fr := NewFakeRunner()
	_, err := fr.Run(context.Background(), CommandRequest{Args: []string{"echo"}})
	if err == nil {
		t.Fatal("expected error for empty result set")
	}
	if !strings.Contains(err.Error(), "no more results") {
		t.Errorf("error should mention exhausted results, got: %v", err)
	}
}

func TestFakeRunnerAddResult(t *testing.T) {
	fr := NewFakeRunner()
	fr.AddResult(contracts.CommandResult{Stdout: "added"})
	res, err := fr.Run(context.Background(), CommandRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Stdout != "added" {
		t.Errorf("stdout = %q, want %q", res.Stdout, "added")
	}
}

func TestFakeRunnerReset(t *testing.T) {
	fr := NewFakeRunner(contracts.CommandResult{Stdout: "to-clear"})
	fr.Reset()
	_, err := fr.Run(context.Background(), CommandRequest{})
	if err == nil {
		t.Fatal("expected error after reset")
	}
}

func TestFakeRunnerRunFunc(t *testing.T) {
	fr := NewFakeRunner()
	fr.RunFunc = func(_ context.Context, req CommandRequest) (*contracts.CommandResult, error) {
		if len(req.Args) > 0 && req.Args[0] == "fail" {
			return nil, errors.New("custom error")
		}
		return &contracts.CommandResult{Stdout: "from-func"}, nil
	}

	// Success path
	res, err := fr.Run(context.Background(), CommandRequest{Args: []string{"ok"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Stdout != "from-func" {
		t.Errorf("stdout = %q, want %q", res.Stdout, "from-func")
	}

	// Error path
	_, err = fr.Run(context.Background(), CommandRequest{Args: []string{"fail"}})
	if err == nil {
		t.Fatal("expected error from RunFunc")
	}
	if !strings.Contains(err.Error(), "custom error") {
		t.Errorf("expected custom error, got: %v", err)
	}
}

func TestFakeRunnerTeardownResult(t *testing.T) {
	fr := NewFakeRunner(contracts.CommandResult{
		Stdout:    "partial",
		ExitCode:  -1,
		TornDown:  true,
		TimedOut:  true,
		Signaled:  true,
		OOMKilled: false,
	})

	_, err := fr.Run(context.Background(), CommandRequest{})
	if err == nil {
		t.Fatal("expected error for torn-down process")
	}
	var cmdErr *CommandResultError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("expected *CommandResultError, got %T: %v", err, err)
	}
	if !cmdErr.Result.TornDown {
		t.Error("expected TornDown flag")
	}
	if !cmdErr.Result.TimedOut {
		t.Error("expected TimedOut flag")
	}
	if !cmdErr.Result.Signaled {
		t.Error("expected Signaled flag")
	}
}

func TestFakeRunnerDurationSleep(t *testing.T) {
	start := time.Now()
	fr := NewFakeRunner(contracts.CommandResult{
		Stdout:   "slow",
		Duration: 50 * time.Millisecond,
	})
	_, err := fr.Run(context.Background(), CommandRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("expected at least 50ms sleep, got %v", elapsed)
	}
}

func TestFakeRunnerImplementsCommandRunner(t *testing.T) {
	var r CommandRunner = NewFakeRunner()
	if r == nil {
		t.Fatal("FakeRunner should implement CommandRunner")
	}
}

func TestFakeRunnerConcurrentSafety(t *testing.T) {
	fr := NewFakeRunner(
		contracts.CommandResult{Stdout: "a"},
		contracts.CommandResult{Stdout: "b"},
		contracts.CommandResult{Stdout: "c"},
	)
	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		fr.Run(ctx, CommandRequest{})
		done <- struct{}{}
	}()
	go func() {
		fr.Run(ctx, CommandRequest{})
		done <- struct{}{}
	}()
	go func() {
		fr.Run(ctx, CommandRequest{})
		done <- struct{}{}
	}()
	for i := 0; i < 3; i++ {
		<-done
	}
	_, err := fr.Run(ctx, CommandRequest{})
	if err == nil {
		t.Error("expected out-of-results after concurrent consumption")
	}
}
