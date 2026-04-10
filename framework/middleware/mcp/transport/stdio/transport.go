package stdio

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/lexcodex/relurpify/framework/sandbox"
)

type Config struct {
	Command string
	Args    []string
	Dir     string
	Env     []string
	Policy  sandbox.CommandPolicy
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Command) == "" {
		return fmt.Errorf("command required")
	}
	return nil
}

type Process interface {
	Stdin() io.WriteCloser
	Stdout() io.ReadCloser
	Stderr() io.ReadCloser
	PID() int
	Wait() error
	Kill() error
}

type Launcher interface {
	Launch(ctx context.Context, cfg Config) (Process, error)
}

type Transport struct {
	process Process
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser

	waitCh    chan error
	closeOnce sync.Once
	closeErr  error
}

func Open(ctx context.Context, launcher Launcher, cfg Config) (*Transport, error) {
	if launcher == nil {
		launcher = execLauncher{}
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	process, err := launcher.Launch(ctx, cfg)
	if err != nil {
		return nil, err
	}
	t := &Transport{
		process: process,
		stdin:   process.Stdin(),
		stdout:  process.Stdout(),
		stderr:  process.Stderr(),
		waitCh:  make(chan error, 1),
	}
	go func() {
		t.waitCh <- process.Wait()
	}()
	go func() {
		<-ctx.Done()
		_ = t.Close()
	}()
	return t, nil
}

func (t *Transport) Reader() io.Reader {
	if t == nil {
		return nil
	}
	return t.stdout
}

func (t *Transport) Writer() io.Writer {
	if t == nil {
		return nil
	}
	return t.stdin
}

func (t *Transport) Stderr() io.Reader {
	if t == nil {
		return nil
	}
	return t.stderr
}

func (t *Transport) PID() int {
	if t == nil || t.process == nil {
		return 0
	}
	return t.process.PID()
}

func (t *Transport) Wait() error {
	if t == nil {
		return nil
	}
	return <-t.waitCh
}

func (t *Transport) Close() error {
	if t == nil {
		return nil
	}
	t.closeOnce.Do(func() {
		if t.stdin != nil {
			_ = t.stdin.Close()
		}
		t.closeErr = t.process.Kill()
	})
	return t.closeErr
}

type execLauncher struct{}

func (execLauncher) Launch(ctx context.Context, cfg Config) (Process, error) {
	if cfg.Policy != nil {
		if err := cfg.Policy.AllowCommand(ctx, sandbox.CommandRequest{
			Workdir: cfg.Dir,
			Args:    append([]string{cfg.Command}, cfg.Args...),
			Env:     append([]string(nil), cfg.Env...),
		}); err != nil {
			return nil, err
		}
	}
	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	if strings.TrimSpace(cfg.Dir) != "" {
		cmd.Dir = cfg.Dir
	}
	if len(cfg.Env) > 0 {
		cmd.Env = append([]string(nil), cfg.Env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return execProcess{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}, nil
}

type execProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

func (p execProcess) Stdin() io.WriteCloser { return p.stdin }
func (p execProcess) Stdout() io.ReadCloser { return p.stdout }
func (p execProcess) Stderr() io.ReadCloser { return p.stderr }
func (p execProcess) PID() int {
	if p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}
func (p execProcess) Wait() error { return p.cmd.Wait() }
func (p execProcess) Kill() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	err := p.cmd.Process.Kill()
	if err != nil && strings.Contains(err.Error(), "process already finished") {
		return nil
	}
	return err
}
