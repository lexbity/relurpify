package euclotui

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
)

func TestStepperInitialState(t *testing.T) {
	s := NewStepper()
	if s.Current() != PhaseIdle {
		t.Errorf("initial phase = %v, want idle", s.Current())
	}
	if s.Render(theme.Default()) != "" {
		t.Error("idle stepper should render empty")
	}
}

func TestStepperAdvancesOnEvents(t *testing.T) {
	s := NewStepper()

	s.Advance(PhaseIntake)
	if s.Current() != PhaseIntake {
		t.Errorf("after intake = %v, want intake", s.Current())
	}

	s.Advance(PhasePlan)
	if s.Current() != PhasePlan {
		t.Errorf("after plan = %v, want plan", s.Current())
	}

	s.Advance(PhaseExecute)
	if s.Current() != PhaseExecute {
		t.Errorf("after execute = %v, want execute", s.Current())
	}
}

func TestStepperComplete(t *testing.T) {
	s := NewStepper()
	s.Advance(PhaseIntake)
	s.Advance(PhasePlan)
	s.Advance(PhaseExecute)
	s.Advance(PhaseVerify)
	s.Complete()

	if s.Current() != PhaseDone {
		t.Errorf("after complete = %v, want done", s.Current())
	}
}

func TestStepperIdempotent(t *testing.T) {
	s := NewStepper()
	s.Advance(PhaseIntake)
	s.Advance(PhaseIntake) // second advance to same phase
	if s.Current() != PhaseIntake {
		t.Errorf("after duplicate = %v, still want intake", s.Current())
	}
}

func TestStepperNoRegression(t *testing.T) {
	s := NewStepper()
	s.Advance(PhaseExecute)
	s.Advance(PhasePlan) // should be no-op (regression)
	if s.Current() != PhaseExecute {
		t.Errorf("after regression = %v, want execute", s.Current())
	}
}

func TestStepperReset(t *testing.T) {
	s := NewStepper()
	s.Advance(PhaseExecute)
	s.Reset()
	if s.Current() != PhaseIdle {
		t.Errorf("after reset = %v, want idle", s.Current())
	}
}

func TestStepperNilSafe(t *testing.T) {
	var s *Stepper
	s.Advance(PhaseIntake) // should not panic
	if s.Current() != PhaseIdle {
		t.Error("nil stepper should report idle")
	}
	if s.Render(theme.Default()) != "" {
		t.Error("nil stepper should render empty")
	}
	s.Reset() // should not panic
}

func TestStepperRenderShowsProgression(t *testing.T) {
	s := NewStepper()
	th := theme.Default()

	s.Advance(PhaseIntake)
	r := s.Render(th)
	if !strings.Contains(r, "intake") {
		t.Errorf("render missing intake phase: %s", r)
	}
	if !strings.Contains(r, "plan") {
		t.Errorf("render should show plan as pending: %s", r)
	}

	s.Advance(PhasePlan)
	r = s.Render(th)
	if !strings.Contains(r, "intake") || !strings.Contains(r, "plan") {
		t.Errorf("render should show both phases: %s", r)
	}
}

func TestStepperRenderFullCycle(t *testing.T) {
	s := NewStepper()
	th := theme.Default()

	s.Advance(PhaseIntake)
	s.Advance(PhasePlan)
	s.Advance(PhaseExecute)
	s.Advance(PhaseVerify)
	s.Complete()

	r := s.Render(th)
	for _, phase := range []string{"intake", "plan", "execute", "verify", "done"} {
		if !strings.Contains(r, phase) {
			t.Errorf("render missing %s: %s", phase, r)
		}
	}
}

func TestStepperRenderCompact(t *testing.T) {
	s := NewStepper()
	th := theme.Default()

	if s.RenderCompact(th) != "" {
		t.Error("idle compact render should be empty")
	}

	s.Advance(PhaseExecute)
	compact := s.RenderCompact(th)
	if !strings.Contains(compact, "execute") {
		t.Errorf("compact render = %q, want execute", compact)
	}
}

func TestStepperRenderFromPhases(t *testing.T) {
	th := theme.Default()
	r := renderStepper(th, PhaseExecute)
	if !strings.Contains(r, "intake") || !strings.Contains(r, "plan") || !strings.Contains(r, "execute") {
		t.Errorf("renderStepper missing phases: %s", r)
	}
	if !strings.Contains(r, "verify") {
		t.Errorf("renderStepper should show verify as pending: %s", r)
	}
}

func TestStepperRenderFromPhasesIdle(t *testing.T) {
	if r := renderStepper(theme.Default(), PhaseIdle); r != "" {
		t.Errorf("idle renderStepper should be empty, got: %s", r)
	}
}

func TestEventRouterAdvancesStepper(t *testing.T) {
	router := NewEucloEventRouter()
	if router.stepper == nil {
		t.Fatal("NewEucloEventRouter should create a stepper")
	}

	// Simulate a full recipe run through events.
	router.ApplyExecutionEvent(ExecutionEvent{
		Type: reporting.EventTypeIntakeComplete,
	})
	if router.stepper.Current() != PhaseIntake {
		t.Errorf("after intake event = %v, want intake", router.stepper.Current())
	}

	router.ApplyExecutionEvent(ExecutionEvent{
		Type: reporting.EventTypeRouteSelected,
	})
	if router.stepper.Current() != PhasePlan {
		t.Errorf("after route event = %v, want plan", router.stepper.Current())
	}

	router.ApplyExecutionEvent(ExecutionEvent{
		Type: reporting.EventTypeStepCompletedEuclo,
	})
	if router.stepper.Current() != PhaseExecute {
		t.Errorf("after execute event = %v, want execute", router.stepper.Current())
	}

	router.ApplyExecutionEvent(ExecutionEvent{
		Type: reporting.EventTypeExecutionComplete,
	})
	if router.stepper.Current() != PhaseDone {
		t.Errorf("after complete event = %v, want done", router.stepper.Current())
	}
}
