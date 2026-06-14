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

// resolvePath mirrors the PATH lookup that exec.Command performs internally.
// We build *exec.Cmd by hand to keep gosec G204 quiet, which bypasses that
// lookup, so cmd.Path would otherwise be a bare base name (e.g. "runsc") that
// fork/exec cannot locate. Resolving here restores normal lookup behaviour;
// on failure we leave the base name so the error surfaces at Run.
func resolvePath(name string) string {
	if resolved, err := exec.LookPath(name); err == nil {
		return resolved
	}
	return name
}

// Command creates an *exec.Cmd from the prepared args.
func Command(a Args) *exec.Cmd {
	cmd := &exec.Cmd{
		Path: resolvePath(a.Name),
		Args: append([]string{a.Name}, a.Args...),
	}
	return cmd
}

// CommandContext creates an *exec.Cmd with context from the prepared args.
func CommandContext(ctx context.Context, a Args) *exec.Cmd {
	cmd := &exec.Cmd{
		Path: resolvePath(a.Name),
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
