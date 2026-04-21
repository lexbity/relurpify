package stdio

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"github.com/stretchr/testify/require"
)

type stubProcess struct {
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	waitCh    chan error
	killCount atomic.Int32
	pid       int
	killErr   error
}

func (p *stubProcess) Stdin() io.WriteCloser { return p.stdin }
func (p *stubProcess) Stdout() io.ReadCloser { return p.stdout }
func (p *stubProcess) Stderr() io.ReadCloser { return p.stderr }
func (p *stubProcess) PID() int              { return p.pid }
func (p *stubProcess) Wait() error           { return <-p.waitCh }
func (p *stubProcess) Kill() error {
	p.killCount.Add(1)
	select {
	case p.waitCh <- context.Canceled:
	default:
	}
	if p.killErr != nil {
		return p.killErr
	}
	return nil
}

type stubLauncher struct {
	process Process
	err     error
}

func (l stubLauncher) Launch(context.Context, Config) (Process, error) {
	if l.err != nil {
		return nil, l.err
	}
	return l.process, nil
}

func TestOpenRejectsInvalidConfig(t *testing.T) {
	_, err := Open(context.Background(), stubLauncher{}, Config{})
	require.ErrorContains(t, err, "command required")
}

func TestOpenRejectsByPolicyBeforeLaunch(t *testing.T) {
	policyErr := errors.New("blocked by launcher policy")
	_, err := Open(context.Background(), nil, Config{
		Command: "__missing_binary__",
		Policy: sandbox.CommandPolicyFunc(func(context.Context, sandbox.CommandRequest) error {
			return policyErr
		}),
	})
	require.ErrorIs(t, err, policyErr)
	require.ErrorContains(t, err, "blocked by launcher policy")
}

func TestTransportCloseKillsProcessOnce(t *testing.T) {
	process := &stubProcess{
		stdin:  nopWriteCloser{Writer: io.Discard},
		stdout: io.NopCloser(strings.NewReader("")),
		stderr: io.NopCloser(strings.NewReader("")),
		waitCh: make(chan error, 1),
		pid:    42,
	}
	transport, err := Open(context.Background(), stubLauncher{process: process}, Config{Command: "fixture"})
	require.NoError(t, err)
	require.Equal(t, 42, transport.PID())

	require.NoError(t, transport.Close())
	require.NoError(t, transport.Close())
	require.EqualValues(t, 1, process.killCount.Load())
	require.ErrorIs(t, transport.Wait(), context.Canceled)
}

func TestTransportCancelsWithContext(t *testing.T) {
	process := &stubProcess{
		stdin:  nopWriteCloser{Writer: io.Discard},
		stdout: io.NopCloser(strings.NewReader("")),
		stderr: io.NopCloser(strings.NewReader("")),
		waitCh: make(chan error, 1),
	}
	ctx, cancel := context.WithCancel(context.Background())
	transport, err := Open(ctx, stubLauncher{process: process}, Config{Command: "fixture"})
	require.NoError(t, err)

	cancel()
	require.Eventually(t, func() bool {
		return process.killCount.Load() == 1
	}, time.Second, 10*time.Millisecond)
	require.ErrorIs(t, transport.Wait(), context.Canceled)
}

func TestTransportWaitReturnsProcessExit(t *testing.T) {
	process := &stubProcess{
		stdin:  nopWriteCloser{Writer: io.Discard},
		stdout: io.NopCloser(strings.NewReader("")),
		stderr: io.NopCloser(strings.NewReader("")),
		waitCh: make(chan error, 1),
	}
	exitErr := errors.New("process exited")
	process.waitCh <- exitErr

	transport, err := Open(context.Background(), stubLauncher{process: process}, Config{Command: "fixture"})
	require.NoError(t, err)
	require.ErrorIs(t, transport.Wait(), exitErr)
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

func TestTransportNilCases(t *testing.T) {
	t.Run("nil transport Reader", func(t *testing.T) {
		var tr *Transport
		require.Nil(t, tr.Reader())
	})

	t.Run("nil transport Writer", func(t *testing.T) {
		var tr *Transport
		require.Nil(t, tr.Writer())
	})

	t.Run("nil transport Stderr", func(t *testing.T) {
		var tr *Transport
		require.Nil(t, tr.Stderr())
	})

	t.Run("nil transport PID", func(t *testing.T) {
		var tr *Transport
		require.Equal(t, 0, tr.PID())
	})

	t.Run("nil transport Wait", func(t *testing.T) {
		var tr *Transport
		require.NoError(t, tr.Wait())
	})

	t.Run("nil transport Close", func(t *testing.T) {
		var tr *Transport
		require.NoError(t, tr.Close())
	})

	t.Run("transport with nil process PID", func(t *testing.T) {
		tr := &Transport{}
		require.Equal(t, 0, tr.PID())
	})
}

func TestTransportStderr(t *testing.T) {
	process := &stubProcess{
		stdin:  nopWriteCloser{Writer: io.Discard},
		stdout: io.NopCloser(strings.NewReader("")),
		stderr: io.NopCloser(strings.NewReader("error output")),
		waitCh: make(chan error, 1),
		pid:    42,
	}
	transport, err := Open(context.Background(), stubLauncher{process: process}, Config{Command: "test"})
	require.NoError(t, err)

	stderr := transport.Stderr()
	require.NotNil(t, stderr)

	data, err := io.ReadAll(stderr)
	require.NoError(t, err)
	require.Equal(t, "error output", string(data))
}

func TestConfigValidate(t *testing.T) {
	t.Run("empty command", func(t *testing.T) {
		cfg := Config{}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "command required")
	})

	t.Run("whitespace only command", func(t *testing.T) {
		cfg := Config{Command: "   "}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "command required")
	})

	t.Run("valid command", func(t *testing.T) {
		cfg := Config{Command: "echo"}
		err := cfg.Validate()
		require.NoError(t, err)
	})

	t.Run("command with args", func(t *testing.T) {
		cfg := Config{Command: "/bin/bash", Args: []string{"-c", "echo hello"}}
		err := cfg.Validate()
		require.NoError(t, err)
	})
}

func TestTransportCloseKillError(t *testing.T) {
	process := &stubProcess{
		stdin:   nopWriteCloser{Writer: io.Discard},
		stdout:  io.NopCloser(strings.NewReader("")),
		stderr:  io.NopCloser(strings.NewReader("")),
		waitCh:  make(chan error, 1),
		pid:     42,
		killErr: errors.New("kill failed"),
	}

	launcher := stubLauncher{process: process}
	transport, err := Open(context.Background(), launcher, Config{Command: "test"})
	require.NoError(t, err)

	err = transport.Close()
	require.Error(t, err)
	require.Contains(t, err.Error(), "kill failed")
}

func TestExecLauncherPolicy(t *testing.T) {
	t.Run("policy allows command", func(t *testing.T) {
		policy := sandbox.CommandPolicyFunc(func(ctx context.Context, req sandbox.CommandRequest) error {
			return nil
		})

		launcher := execLauncher{}
		_, err := launcher.Launch(context.Background(), Config{
			Command: "echo",
			Args:    []string{"hello"},
			Dir:     "/tmp",
			Env:     []string{"FOO=bar"},
			Policy:  policy,
		})
		require.NoError(t, err)
	})

	t.Run("policy blocks command", func(t *testing.T) {
		policyErr := errors.New("command not allowed")
		policy := sandbox.CommandPolicyFunc(func(ctx context.Context, req sandbox.CommandRequest) error {
			return policyErr
		})

		launcher := execLauncher{}
		_, err := launcher.Launch(context.Background(), Config{
			Command: "echo",
			Policy:  policy,
		})
		require.ErrorIs(t, err, policyErr)
	})
}
