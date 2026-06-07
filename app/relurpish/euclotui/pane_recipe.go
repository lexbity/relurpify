package euclotui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// TabRecipe is the tab identifier for the Recipe/Workshop pane.
const TabRecipe tui.TabID = "recipe"

// RecipePane renders a full RecipeProjection view — steps, groups, HITL gates,
// dependency edges, tool scopes, and runtime status.
type RecipePane struct {
	router *EucloEventRouter
	th     *theme.Theme
	width  int
}

// NewRecipePane creates a recipe/workshop pane.
func NewRecipePane(router *EucloEventRouter, th *theme.Theme) *RecipePane {
	return &RecipePane{
		router: router,
		th:     th,
		width:  80,
	}
}

// SetSize implements tui.Region1Surface.
func (p *RecipePane) SetSize(w, h int) { p.width = w }

// SetStore implements tui.Region1Surface (no-op).
func (p *RecipePane) SetStore(store *tui.SessionStore) {}

// SetActiveTab implements tui.Region1Surface (no-op).
func (p *RecipePane) SetActiveTab(id tui.TabID) {}

// SetFilter implements tui.Region1Surface (no-op).
func (p *RecipePane) SetFilter(filter string) {}

// Refresh implements tui.Region1Surface (reads live from router).
func (p *RecipePane) Refresh() {}

// Update implements tui.Region1Surface.
func (p *RecipePane) Update(msg tea.Msg) (tui.Region1Surface, tea.Cmd) {
	return p, nil
}

// View implements tui.Region1Surface.
func (p *RecipePane) View() string {
	if p == nil || p.router == nil {
		return ""
	}
	snap := p.router.Snapshot()
	if snap.Recipe == nil {
		return p.th.Dim().Render("No recipe selected.")
	}
	return p.renderRecipe(snap)
}

// Cleanup implements tui.Region1Surface.
func (p *RecipePane) Cleanup() {}

// FocusFilescopes implements tui.Region1Surface (no-op).
func (p *RecipePane) FocusFilescopes() {}

// OpenSecurityGuard implements tui.Region1Surface (no-op).
func (p *RecipePane) OpenSecurityGuard() {}

// OpenAIProvider implements tui.Region1Surface (no-op).
func (p *RecipePane) OpenAIProvider() {}

// OpenKeybindings implements tui.Region1Surface (no-op).
func (p *RecipePane) OpenKeybindings() {}

// OpenDoctor implements tui.Region1Surface (no-op).
func (p *RecipePane) OpenDoctor() {}

// HandleInputSubmit implements tui.Region1Surface.
func (p *RecipePane) HandleInputSubmit(value string) tea.Cmd { return nil }

func (p *RecipePane) renderRecipe(snap EucloProjectionSnapshot) string {
	proj := snap.Recipe
	var b strings.Builder

	// Header
	b.WriteString(p.th.Header().Render("Recipe: "+displayName(proj)) + "\n")

	// Metadata
	var meta []string
	if proj.RouteKind != "" {
		meta = append(meta, "route: "+proj.RouteKind)
	}
	if proj.FamilyID != "" {
		meta = append(meta, "family: "+proj.FamilyID)
	}
	if proj.SelectedRoute != "" {
		meta = append(meta, "selected: "+proj.SelectedRoute)
	}
	if len(meta) > 0 {
		b.WriteString("  " + p.th.Dim().Render(strings.Join(meta, " · ")) + "\n")
	}

	// HITL gates
	if len(proj.HITLGates) > 0 {
		b.WriteString("\n" + p.th.Subhead().Render("HITL Gates") + "\n")
		for _, gate := range proj.HITLGates {
			b.WriteString("  " + p.th.Warning().Render("● "+gate) + "\n")
		}
	}

	// Group topology
	if len(proj.Groups) > 0 {
		b.WriteString("\n" + p.th.Subhead().Render("Groups") + "\n")
		for _, g := range proj.Groups {
			kindGlyph := "■"
			switch g.Kind {
			case "parallel":
				kindGlyph = "‖"
			case "conditional":
				kindGlyph = "◇"
			case "pipeline":
				kindGlyph = "→"
			}
			label := fmt.Sprintf("  %s %s (%s)", kindGlyph, g.GroupID, g.Kind)
			if g.Condition != "" {
				label += " if " + g.Condition
			}
			if g.Merge != "" {
				label += " merge=" + g.Merge
			}
			b.WriteString(p.th.Detail().Render(label) + "\n")
			for _, member := range g.MemberStepIDs {
				b.WriteString("    · " + p.th.Dim().Render(member) + "\n")
			}
		}
	}

	// Steps
	if len(proj.Steps) > 0 {
		b.WriteString("\n" + p.th.Subhead().Render("Steps") + "\n")
		for _, step := range proj.Steps {
			p.renderStep(&b, step, snap.StepRuntime[step.StepID])
		}
	}

	if b.Len() == 0 {
		return p.th.Dim().Render("(empty recipe)")
	}
	return p.th.Panel().Render(strings.TrimSpace(b.String()))
}

func displayName(proj *surface.RecipeProjection) string {
	if proj.Name != "" {
		return proj.Name
	}
	return proj.RecipeID
}

func (p *RecipePane) renderStep(b *strings.Builder, step surface.ProjectedStep, rt surface.StepRuntime) {
	glyph := stepGlyphForPane(step)
	sg := statusGlyphForPane(rt.Status)
	paradigmStr := step.Paradigm
	if paradigmStr == "" {
		paradigmStr = step.Type
	}

	label := fmt.Sprintf("%s %s %s  %s", sg, glyph, paradigmStr, step.Goal)

	var annot []string
	if step.CapabilityID != "" {
		annot = append(annot, "cap: "+step.CapabilityID)
	}
	if step.HITL != "" {
		annot = append(annot, "HITL: "+step.HITL)
	}
	if len(step.ToolScopes) > 0 {
		annot = append(annot, "tools: "+strings.Join(step.ToolScopes, ", "))
	}
	if len(step.DependsOn) > 0 {
		annot = append(annot, "deps: "+strings.Join(step.DependsOn, ", "))
	}
	if step.GroupID != "" {
		annot = append(annot, "group: "+step.GroupID)
	}
	if step.Optional {
		annot = append(annot, "optional")
	}
	if rt.DurationMs > 0 {
		annot = append(annot, fmt.Sprintf("%dms", rt.DurationMs))
	}
	if rt.Err != "" {
		annot = append(annot, "error: "+rt.Err)
	}

	var style lipgloss.Style
	switch rt.Status {
	case surface.StepActive:
		style = p.th.Active()
	case surface.StepDone:
		style = p.th.Success()
	case surface.StepFailed:
		style = p.th.Error()
	case surface.StepSkipped:
		style = p.th.Pending()
	default:
		style = p.th.Body()
	}

	b.WriteString("  " + style.Render(label) + "\n")
	if len(annot) > 0 {
		b.WriteString("    " + p.th.Dim().Render(strings.Join(annot, " · ")) + "\n")
	}
	b.WriteString("    " + p.th.Detail().Render(step.StepID) + "\n")
}

func stepGlyphForPane(step surface.ProjectedStep) string {
	if step.Paradigm != "" {
		return theme.Default().ParadigmGlyph(surface.Paradigm(step.Paradigm))
	}
	return "??"
}

func statusGlyphForPane(status surface.StepStatus) string {
	switch status {
	case surface.StepActive:
		return "●"
	case surface.StepDone:
		return "✓"
	case surface.StepFailed:
		return "✗"
	case surface.StepSkipped:
		return "░"
	default:
		return "○"
	}
}
