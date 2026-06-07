package testsupport

import (
	"context"
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
)

var _ sandbox.CommandRunner = (*FakeCommandRunner)(nil)

func TestFakeRunner_ReturnsCannedResponse(t *testing.T) {
	t.Parallel()

	fr := FakeRunner(FakeResponse{
		Stdout: "hello world",
		Stderr: "",
	})

	res, err := fr.Run(context.Background(), sandbox.CommandRequest{
		Args: []string{"echo", "hello"},
	})
	if res.Stdout != "hello world" {
		t.Errorf("stdout = %q, want %q", res.Stdout, "hello world")
	}
	if res.Stderr != "" {
		t.Errorf("stderr = %q, want empty", res.Stderr)
	}
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}

func TestFakeRunner_ReturnsError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("command failed")
	fr := FakeRunner(FakeResponse{
		Stdout: "",
		Err:    expectedErr,
	})

	_, err := fr.Run(context.Background(), sandbox.CommandRequest{
		Args: []string{"false"},
	})
	if !errors.Is(err, expectedErr) {
		t.Errorf("err = %v, want %v", err, expectedErr)
	}
}

func TestFakeRunner_RecordsCalls(t *testing.T) {
	t.Parallel()

	fr := FakeRunner(FakeResponse{Stdout: "ok"})

	fr.Run(context.Background(), sandbox.CommandRequest{Args: []string{"echo", "a"}})
	fr.Run(context.Background(), sandbox.CommandRequest{Args: []string{"echo", "b"}})

	if fr.CallCount() != 2 {
		t.Errorf("CallCount = %d, want 2", fr.CallCount())
	}

	last := fr.LastCall()
	if last == nil {
		t.Fatal("LastCall returned nil")
	}
	if len(last.Args) != 2 || last.Args[1] != "b" {
		t.Errorf("last args = %v, want [echo b]", last.Args)
	}
}

func TestFakeRunner_ResponseMatchingByArgs(t *testing.T) {
	t.Parallel()

	fr := FakeRunner(
		FakeResponse{
			MatchArgs: []string{"echo", "hello"},
			Stdout:    "hi there",
		},
		FakeResponse{
			MatchArgs: []string{"echo"},
			Stdout:    "generic echo",
		},
	)

	// Exact prefix match
	res, _ := fr.Run(context.Background(), sandbox.CommandRequest{
		Args: []string{"echo", "hello", "world"},
	})
	if res.Stdout != "hi there" {
		t.Errorf("stdout = %q, want %q", res.Stdout, "hi there")
	}

	// Broader prefix match (second response)
	res, _ = fr.Run(context.Background(), sandbox.CommandRequest{
		Args: []string{"echo", "foo"},
	})
	if res.Stdout != "generic echo" {
		t.Errorf("stdout = %q, want %q", res.Stdout, "generic echo")
	}

	// No match — default fallback
	res, _ = fr.Run(context.Background(), sandbox.CommandRequest{
		Args: []string{"ls"},
	})
	if res.Stdout != "" {
		t.Errorf("stdout = %q, want empty", res.Stdout)
	}
}

func TestFakeRunner_NoMatchReturnsEmpty(t *testing.T) {
	t.Parallel()

	fr := FakeRunner(FakeResponse{
		MatchArgs: []string{"git"},
		Stdout:    "git output",
	})

	res, err := fr.Run(context.Background(), sandbox.CommandRequest{
		Args: []string{"echo", "hello"},
	})
	if res.Stdout != "" {
		t.Errorf("stdout = %q, want empty", res.Stdout)
	}
	if res.Stderr != "" {
		t.Errorf("stderr = %q, want empty", res.Stderr)
	}
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}

func TestFakeRunner_Reset(t *testing.T) {
	t.Parallel()

	fr := FakeRunner(FakeResponse{Stdout: "a"})
	fr.Run(context.Background(), sandbox.CommandRequest{Args: []string{"echo"}})
	if fr.CallCount() != 1 {
		t.Fatal("expected 1 call before reset")
	}

	fr.Reset()
	if fr.CallCount() != 0 {
		t.Errorf("CallCount = %d after reset, want 0", fr.CallCount())
	}
	if fr.LastCall() != nil {
		t.Error("LastCall should be nil after reset")
	}
}

func TestFakeRunner_NilSafe(t *testing.T) {
	t.Parallel()

	var fr *FakeCommandRunner
	if fr.CallCount() != 0 {
		t.Error("CallCount on nil should be 0")
	}
	if fr.LastCall() != nil {
		t.Error("LastCall on nil should be nil")
	}
	fr.Reset() // should not panic
	_, err := fr.Run(context.Background(), sandbox.CommandRequest{})
	if err == nil {
		t.Error("Run on nil should return error")
	}
}
