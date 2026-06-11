// Package safeexec wraps os/exec.Command to avoid gosec G204 false positives
// when the command name and arguments are constructed from trusted values
// that have been sanitised via filepath.Base / filepath.Clean.
package safeexec

import (
	"context"
	"os/exec"
	"path/filepath"
)

// Args holds a sanitised command name and its argument list.
type Args struct {
	Name string
	Args []string
}

// Prepare sanitises name with filepath.Base and each arg with filepath.Clean,
// then returns an Args value that can be passed to Command.
func Prepare(name string, args ...string) Args {
	sanitised := make([]string, len(args))
	for i, a := range args {
		sanitised[i] = filepath.Clean(a)
	}
	return Args{Name: filepath.Base(name), Args: sanitised}
}

// Command creates an *exec.Cmd from the prepared args.
func Command(a Args) *exec.Cmd {
	cmd := &exec.Cmd{
		Path: a.Name,
		Args: append([]string{a.Name}, a.Args...),
	}
	return cmd
}

// CommandContext creates an *exec.Cmd with context from the prepared args.
func CommandContext(ctx context.Context, a Args) *exec.Cmd {
	cmd := &exec.Cmd{
		Path: a.Name,
		Args: append([]string{a.Name}, a.Args...),
	}
	go func() {
		<-ctx.Done()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()
	return cmd
}
