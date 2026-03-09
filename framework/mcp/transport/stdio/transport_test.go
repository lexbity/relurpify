package stdio

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stubProcess struct {
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	waitCh    chan error
	killCount atomic.Int32
	pid       int
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
