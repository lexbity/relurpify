package euclotui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// Stepper renders a two-tier progress view:
//
//	Tier 1 — Macro lifecycle rail (idle → intake → route → execute → verify → done)
//	Tier 2 — Dynamic recipe-step graph (one node per real step, paradigm-labelled,
//	          with live runtime status and parallel group topology).
type Stepper struct {
	recipe      *surface.RecipeProjection
	stepRuntime map[string]surface.StepRuntime
	macro       surface.MacroPhase
}

// NewStepper creates a stepper from the current projection snapshot.
func NewStepper(recipe *surface.RecipeProjection, stepRuntime map[string]surface.StepRuntime, macro surface.MacroPhase) *Stepper {
	runtime := stepRuntime
	if runtime == nil {
		runtime = make(map[string]surface.StepRuntime)
	}
	return &Stepper{
		recipe:      recipe,
		stepRuntime: runtime,
		macro:       macro,
	}
}

// macroOrder defines the canonical lifecycle phases in order.
var macroOrder = []surface.MacroPhase{
	surface.MacroIntake,
	surface.MacroRoute,
	surface.MacroExecute,
	surface.MacroVerify,
	surface.MacroDone,
}

// Render produces the two-tier output: a macro lifecycle rail followed by
// the dynamic step graph.
func (s *Stepper) Render(th *theme.Theme) string {
	if s == nil || th == nil {
		return ""
	}
	if s.macro == surface.MacroIdle {
		return ""
	}
	var b strings.Builder

	// Tier 1: Macro lifecycle rail.
	b.WriteString(renderMacroRail(th, s.macro))

	// Tier 2: Dynamic step graph.
	if s.recipe != nil && len(s.recipe.Steps) > 0 {
		b.WriteString("\n")
		for _, step := range s.recipe.Steps {
			b.WriteString("\n  " + renderProjectedStep(th, step, s.stepRuntime[step.StepID]))
		}
	}
	return b.String()
}

func renderMacroRail(th *theme.Theme, current surface.MacroPhase) string {
	parts := make([]string, 0, len(macroOrder))
	for _, p := range macroOrder {
		label := macroPhaseLabel(p)
		var style lipgloss.Style
		if p == current {
			style = th.Active()
		} else if p.Before(current) {
			style = th.Success()
		} else {
			style = th.Pending()
		}
		parts = append(parts, style.Render(label))
	}
	return strings.Join(parts, " → ")
}

func renderProjectedStep(th *theme.Theme, ps surface.ProjectedStep, rt surface.StepRuntime) string {
	glyph := stepGlyph(ps, rt)
	label := ps.Paradigm
	if label == "" {
		label = ps.Type
	}
	goal := ps.Goal
	if goal == "" {
		goal = ps.StepID
	}

	var style lipgloss.Style
	switch rt.Status {
	case surface.StepActive:
		style = th.Active()
	case surface.StepDone:
		style = th.Success()
	case surface.StepFailed:
		style = th.Error()
	case surface.StepSkipped:
		style = th.Pending()
	default:
		style = th.Dim()
	}

	parts := []string{glyph, label + ":", goal}
	if rt.Total > 0 {
		parts = append(parts, fmt.Sprintf("(%d/%d)", rt.Index+1, rt.Total))
	}
	return style.Render(strings.Join(parts, " "))
}

func stepGlyph(ps surface.ProjectedStep, rt surface.StepRuntime) string {
	if ps.Paradigm != "" {
		return theme.Default().ParadigmGlyph(surface.Paradigm(ps.Paradigm))
	}
	return "??"
}

func macroPhaseLabel(p surface.MacroPhase) string {
	switch p {
	case surface.MacroIntake:
		return "intake"
	case surface.MacroRoute:
		return "route"
	case surface.MacroExecute:
		return "execute"
	case surface.MacroVerify:
		return "verify"
	case surface.MacroDone:
		return "done"
	default:
		return p.String()
	}
}
