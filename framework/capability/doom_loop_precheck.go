package capability

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// DoomLoopPrecheck wraps a DoomLoopDetector as both an InvocationPrecheck
// (checked before each call) and a PostInvocationHook (recording the result
// after each call). A single detector is shared between two adapter
// registrations — one AddPrecheck, one AddPostcheck.
type DoomLoopPrecheck struct {
	detector *DoomLoopDetector
}

// NewDoomLoopPrecheck creates a precheck/postcheck pair for the registry.
func NewDoomLoopPrecheck() DoomLoopPrecheck {
	return DoomLoopPrecheck{
		detector: NewDoomLoopDetector(DefaultDoomLoopConfig()),
	}
}

// Check implements InvocationPrecheck. It runs the detector before the call
// and blocks if a doom loop is detected.
func (p DoomLoopPrecheck) Check(desc core.CapabilityDescriptor, args map[string]any) error {
	if p.detector == nil {
		return nil
	}
	if err := p.detector.Check(desc, args); err != nil {
		var doomErr *DoomLoopError
		if !asDoomLoopError(err, &doomErr) {
			return err
		}
		return &actionableDoomLoopError{inner: doomErr, desc: desc}
	}
	return nil
}

// Record implements PostInvocationHook. It feeds the completed call's result
// into the detector so future prechecks can detect patterns.
func (p DoomLoopPrecheck) Record(desc core.CapabilityDescriptor, result *contracts.ToolResult) error {
	if p.detector == nil {
		return nil
	}
	return p.detector.RecordResult(desc, result)
}

// Compile-time interface checks.
var (
	_ InvocationPrecheck  = DoomLoopPrecheck{}
	_ PostInvocationHook  = DoomLoopPrecheck{}
)

// asDoomLoopError unwraps a *DoomLoopError from err, including wrapped errors.
func asDoomLoopError(err error, target **DoomLoopError) bool {
	if err == nil {
		return false
	}
	var dle *DoomLoopError
	if as, ok := err.(interface{ As(interface{}) bool }); ok {
		if as.As(&dle) {
			*target = dle
			return true
		}
	}
	return false
}

// actionableDoomLoopError wraps a DoomLoopError with a human-readable message
// that tells the model what went wrong and how to recover.
type actionableDoomLoopError struct {
	inner *DoomLoopError
	desc  core.CapabilityDescriptor
}

func (e *actionableDoomLoopError) Error() string {
	if e == nil || e.inner == nil {
		return "tool execution stopped: repeated calls detected. Try a different approach."
	}
	name := e.desc.Name
	if name == "" {
		name = "tool"
	}
	switch e.inner.Kind {
	case DoomLoopIdenticalCall:
		return fmt.Sprintf("tool execution stopped: %d identical calls to %q did not make progress. Try a different approach or a different tool.", e.inner.CallCount, name)
	case DoomLoopOscillating:
		return fmt.Sprintf("tool execution stopped: oscillating between two tools for %d calls. Choose one approach and commit to it.", e.inner.CallCount)
	case DoomLoopErrorFixation:
		return fmt.Sprintf("tool execution stopped: %s keeps failing after %d attempts. The error is not going away — try a different strategy.", name, e.inner.CallCount)
	case DoomLoopProgressStall:
		return fmt.Sprintf("tool execution stopped: no progress detected after %d calls. Try a fundamentally different approach.", e.inner.CallCount)
	default:
		return fmt.Sprintf("tool execution stopped: repeated calls to %q are not making progress. Try a different approach.", name)
	}
}

// Unwrap exposes the inner DoomLoopError for error matching.
func (e *actionableDoomLoopError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.inner
}
