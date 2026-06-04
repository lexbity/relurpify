package euclotui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
)

// Phase represents a named step in the Euclo recipe lifecycle.
type Phase int

const (
	PhaseIdle     Phase = iota // No recipe running
	PhaseIntake                // Understanding request, gathering context
	PhasePlan                  // Formulating approach
	PhaseExecute               // Making changes
	PhaseVerify                // Checking results
	PhaseDone                  // Recipe completed
)

func (p Phase) String() string {
	switch p {
	case PhaseIdle:
		return "idle"
	case PhaseIntake:
		return "intake"
	case PhasePlan:
		return "plan"
	case PhaseExecute:
		return "execute"
	case PhaseVerify:
		return "verify"
	case PhaseDone:
		return "done"
	default:
		return "unknown"
	}
}

// Stepper renders the recipe phase progression as a compact visual bar.
// It is driven by milestone events emitted from the Euclo event router.
type Stepper struct {
	phases []Phase
}

// NewStepper creates a stepper initialised at idle.
func NewStepper() *Stepper {
	return &Stepper{
		phases: []Phase{PhaseIdle},
	}
}

// Advance moves the stepper to the next phase. No-op if the phase is already
// at or beyond the given phase (idempotent).
func (s *Stepper) Advance(to Phase) {
	if s == nil {
		return
	}
	if len(s.phases) == 0 {
		s.phases = []Phase{to}
		return
	}
	last := s.phases[len(s.phases)-1]
	if to <= last {
		return
	}
	s.phases = append(s.phases, to)
}

// Current returns the latest phase.
func (s *Stepper) Current() Phase {
	if s == nil || len(s.phases) == 0 {
		return PhaseIdle
	}
	return s.phases[len(s.phases)-1]
}

// Complete marks the recipe as done.
func (s *Stepper) Complete() {
	s.Advance(PhaseDone)
}

// Reset returns the stepper to idle.
func (s *Stepper) Reset() {
	if s != nil {
		s.phases = []Phase{PhaseIdle}
	}
}

// Render produces a compact visual bar of the phase progression using the
// given theme. Each phase is rendered with its corresponding role style:
// Active for the current phase, Success for completed, Pending for future.
func (s *Stepper) Render(th *theme.Theme) string {
	if s == nil || th == nil {
		return ""
	}
	if s.Current() == PhaseIdle {
		return ""
	}

	ordered := []Phase{PhaseIntake, PhasePlan, PhaseExecute, PhaseVerify, PhaseDone}
	parts := make([]string, 0, len(ordered))
	current := s.Current()

	for _, p := range ordered {
		label := p.String()
		var style lipgloss.Style
		if p < current {
			style = th.Success()
		} else if p == current {
			style = th.Active()
		} else {
			style = th.Pending()
		}
		parts = append(parts, style.Render(label))
	}
	return strings.Join(parts, " → ")
}

// RenderCompact produces a single-line summary for use in constrained space.
func (s *Stepper) RenderCompact(th *theme.Theme) string {
	if s == nil || th == nil || s.Current() == PhaseIdle {
		return ""
	}
	return th.Active().Render(s.Current().String())
}
