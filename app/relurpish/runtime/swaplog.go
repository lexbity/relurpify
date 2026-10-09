package runtime

import (
	"io"
	"os"
	"sync/atomic"
)

// StandardLogWriter is the swappable writer the process's stdlib logger writes
// through. ReloadWorkspace repoints its target at the new runtime's log file,
// so the global logger never writes into a closed file after a workspace
// reload.
//
// The swap writer itself never closes: closing the file behind each target is
// the owning runtime's business (its Workspace.Close). The target is read by
// every Write, so a reader either sees the old target or the new one — never a
// partially-`swapped Writer.
type StandardLogWriter struct {
	target atomic.Value // io.Writer
}

// NewStandardLogWriter constructs a swappable writer that begins writing to
// initial. A nil initial target resolves to stderr.
func NewStandardLogWriter(initial io.Writer) *StandardLogWriter {
	w := &StandardLogWriter{}
	w.Set(initial)
	return w
}

// Set atomically retargets the writer. A nil target resolves to stderr.
func (w *StandardLogWriter) Set(target io.Writer) {
	if w == nil {
		return
	}
	if target == nil {
		target = io.Writer(os.Stderr)
	}
	w.target.Store(target)
}

// Write implements io.Writer against the current target.
func (w *StandardLogWriter) Write(p []byte) (int, error) {
	if w == nil {
		return 0, nil
	}
	target, _ := w.target.Load().(io.Writer)
	if target == nil {
		return 0, nil
	}
	return target.Write(p)
}

// stdlibLogWriter is the process-wide swappable target the stdlib logger is
// wired to by main; the runtime reload path retargets it. Package-level state
// is sanctioned here because the process's stdlib logger IS global — a global
// logger requires a global swap channel.
var stdlibLogWriter = NewStandardLogWriter(os.Stderr) //nolint:gochecknoglobals // stdlib log writer swap channel for the global logger

// StdlibLogWriter returns the process-wide swappable log writer.
func StdlibLogWriter() *StandardLogWriter {
	return stdlibLogWriter
}

// RepointStdlibLog retargets the process-wide swappable log writer at a
// runtime's workspace log file. It is a no-op when the workspace has no
// logger, so degraded runtimes keep the previous target.
func RepointStdlibLog(rt *Runtime) {
	if rt == nil {
		return
	}
	ws := rt.AgentWorkspace()
	if ws == nil || ws.Logger == nil {
		return
	}
	StdlibLogWriter().Set(ws.Logger.Writer())
}
