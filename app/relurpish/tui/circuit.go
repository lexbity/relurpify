package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// ExecutionReader supplies observed execution facts to the compact canvas.
// Providers must return detached snapshots. A blank status means unobserved;
// the frontend never derives completion from graph order or elapsed time.
type ExecutionReader interface {
	ExecutionSnapshot() CircuitSnapshot
}

type CircuitSnapshot struct {
	RecipeID string
	Name     string
	Route    string
	Steps    []CircuitStep
	Notice   string
}

type CircuitStep struct {
	ID           string
	Strategy     string
	Goal         string
	Capability   string
	Dependencies []string
	ToolScopes   []string
	Group        string
	Gate         string
	Status       string
	DurationMS   int64
	Error        string
}

// circuitCanvas stores interaction state, never execution authority. Selection
// follows an identity across updates; new progress cannot move reading focus.
type circuitCanvas struct {
	snapshot CircuitSnapshot
	selected string
	inspect  bool
	offset   int
}

func (c *circuitCanvas) apply(snapshot CircuitSnapshot) {
	if snapshot.RecipeID != c.snapshot.RecipeID {
		c.selected, c.inspect, c.offset = "", false, 0
	}
	snapshot.Steps = slices.Clone(snapshot.Steps)
	for i := range snapshot.Steps {
		snapshot.Steps[i].Dependencies = slices.Clone(snapshot.Steps[i].Dependencies)
		snapshot.Steps[i].ToolScopes = slices.Clone(snapshot.Steps[i].ToolScopes)
	}
	c.snapshot = snapshot
	if c.index() < 0 && len(snapshot.Steps) > 0 {
		c.selected = snapshot.Steps[0].ID
	}
}

func (c circuitCanvas) index() int {
	return slices.IndexFunc(c.snapshot.Steps, func(step CircuitStep) bool { return step.ID == c.selected })
}

func (c *circuitCanvas) key(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "up", "down", "home", "end":
		if c.inspect {
			delta := 1
			if msg.String() == "up" {
				delta = -1
			}
			c.offset = max(0, c.offset+delta)
			if msg.String() == "home" {
				c.offset = 0
			}
			return true
		}
		if len(c.snapshot.Steps) == 0 {
			return true
		}
		i := max(0, c.index())
		switch msg.String() {
		case "up":
			i = max(0, i-1)
		case "down":
			i = min(len(c.snapshot.Steps)-1, i+1)
		case "home":
			i = 0
		case "end":
			i = len(c.snapshot.Steps) - 1
		}
		c.selected = c.snapshot.Steps[i].ID
		return true
	case "enter":
		if c.index() >= 0 {
			c.inspect, c.offset = true, 0
		}
		return true
	case "esc":
		if c.inspect {
			c.inspect, c.offset = false, 0
			return true
		}
	}
	return false
}

func (c circuitCanvas) view(width, rows int, focused bool) string {
	if c.inspect && c.index() >= 0 {
		return c.inspector(width, rows)
	}
	name := c.snapshot.Name
	if name == "" {
		name = c.snapshot.RecipeID
	}
	if name == "" {
		if c.snapshot.Notice != "" {
			return boundedScreen("Execution\n\n"+terminalText(c.snapshot.Notice), width, rows)
		}
		return "Execution\n\nNo observed recipe yet. Submit a request in the bar.\n\nThe circuit appears when Euclo reports its selected recipe."
	}
	lines := []string{"Execution / " + terminalText(name), ""}
	// Reserve a fixed row per step. Activity changes its marker, not geometry.
	budget := max(1, rows-5)
	start := max(0, c.index()-budget+1)
	for _, step := range c.snapshot.Steps[start:min(len(c.snapshot.Steps), start+budget)] {
		selection := " "
		if step.ID == c.selected {
			selection = ">"
		}
		marker, status := circuitStatus(step.Status)
		strategy := step.Strategy
		if strategy == "" {
			strategy = step.Capability
		}
		lines = append(lines, fmt.Sprintf("%s [%s] %-16s %s · %s", selection, marker, terminalText(step.ID), terminalText(strategy), status))
	}
	if len(c.snapshot.Steps) == 0 {
		lines = append(lines, "No step observations in this projection.")
	}
	if len(c.snapshot.Steps) > budget {
		lines = append(lines, fmt.Sprintf("  %d–%d of %d steps", start+1, min(len(c.snapshot.Steps), start+budget), len(c.snapshot.Steps)))
	}
	lines = append(lines, "")
	if focused {
		lines = append(lines, "↑/↓ select · Enter inspect · Escape returns to bar")
	} else {
		lines = append(lines, "F6 selects steps · /inspect opens detail · /chat returns")
	}
	return boundedScreen(strings.Join(lines, "\n"), width, rows)
}

func (c circuitCanvas) inspector(width, rows int) string {
	step := c.snapshot.Steps[c.index()]
	_, status := circuitStatus(step.Status)
	lines := []string{"Execution / " + terminalText(step.ID), "", "Observed state: " + status}
	for _, field := range [][2]string{
		{"Strategy", step.Strategy}, {"Capability", step.Capability}, {"Goal", step.Goal},
		{"Depends on", strings.Join(step.Dependencies, ", ")}, {"Tool scopes", strings.Join(step.ToolScopes, ", ")},
		{"Group", step.Group}, {"Human gate", step.Gate}, {"Error", step.Error},
	} {
		if field[1] != "" {
			lines = append(lines, field[0]+": "+terminalText(field[1]))
		}
	}
	if step.DurationMS > 0 {
		lines = append(lines, fmt.Sprintf("Reported duration: %d ms", step.DurationMS))
	}
	if c.snapshot.Notice != "" {
		lines = append(lines, "", terminalText(c.snapshot.Notice))
	}
	lines = append(lines, "", "Escape closes inspector · ↑/↓ scroll · bar still requests Euclo")
	wrapped := strings.Split(ansi.Hardwrap(strings.Join(lines, "\n"), max(1, width), true), "\n")
	start := min(c.offset, max(0, len(wrapped)-rows))
	return boundedScreen(strings.Join(wrapped[start:], "\n"), width, rows)
}

func circuitStatus(status string) (string, string) {
	switch status {
	case "active":
		return "*", "active"
	case "done":
		return "+", "completed"
	case "failed":
		return "!", "failed"
	case "skipped":
		return "-", "skipped"
	case "pending":
		return ".", "pending"
	default:
		return "?", "not observed"
	}
}

func (m *RootModel) refreshCircuit() {
	if m.conversation != nil {
		m.circuit.apply(m.frontendState.Circuit)
		return
	}
	if reader, ok := m.activeSurface.(ExecutionReader); ok {
		m.circuit.apply(reader.ExecutionSnapshot())
	} else {
		m.circuit.apply(CircuitSnapshot{})
	}
}

func rootHandleRun(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) != 0 {
		m.addSystemMessage("Usage: /run")
		return m, nil
	}
	m.refreshCircuit()
	m.setActiveTab(TabRun)
	return m, nil
}

func rootHandleInspect(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) > 1 {
		m.addSystemMessage("Usage: /inspect [step-id]")
		return m, nil
	}
	m.refreshCircuit()
	if len(args) == 1 {
		if !slices.ContainsFunc(m.circuit.snapshot.Steps, func(step CircuitStep) bool { return step.ID == args[0] }) {
			m.addSystemMessage("No observed step with ID " + args[0])
			return m, nil
		}
		m.circuit.selected = args[0]
	}
	m.circuit.inspect, m.circuit.offset = m.circuit.index() >= 0, 0
	m.setActiveTab(TabRun)
	m.setFocus(FocusRegionRegion1)
	return m, nil
}
