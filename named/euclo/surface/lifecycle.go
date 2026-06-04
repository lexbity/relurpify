package surface

// MacroPhase represents a coarse lifecycle phase in the Euclo recipe execution.
type MacroPhase int

const (
	MacroIdle    MacroPhase = iota // No recipe running
	MacroIntake                    // Understanding request, gathering context
	MacroRoute                     // Selecting recipe/family/route
	MacroExecute                   // Executing recipe steps
	MacroVerify                    // Checking results
	MacroDone                      // Recipe completed
)

// String returns the lowercase label for a macro phase.
func (p MacroPhase) String() string {
	switch p {
	case MacroIdle:
		return "idle"
	case MacroIntake:
		return "intake"
	case MacroRoute:
		return "route"
	case MacroExecute:
		return "execute"
	case MacroVerify:
		return "verify"
	case MacroDone:
		return "done"
	default:
		return "unknown"
	}
}

// Before reports whether p occurs strictly before other in the lifecycle.
func (p MacroPhase) Before(other MacroPhase) bool {
	return p >= MacroIdle && p <= MacroDone && p < other
}

// After reports whether p occurs strictly after other in the lifecycle.
func (p MacroPhase) After(other MacroPhase) bool {
	return p >= MacroIdle && p <= MacroDone && p > other
}
