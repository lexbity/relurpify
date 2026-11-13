package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestV2CircuitSelectionSurvivesProgress(t *testing.T) {
	c := circuitCanvas{}
	snapshot := CircuitSnapshot{RecipeID: "repair", Steps: []CircuitStep{
		{ID: "read", Strategy: "analyze"},
		{ID: "edit", Dependencies: []string{"read"}},
		{ID: "verify", Dependencies: []string{"edit"}},
	}}
	c.apply(snapshot)
	c.key(tea.KeyMsg{Type: tea.KeyDown})
	c.key(tea.KeyMsg{Type: tea.KeyEnter})
	snapshot.Steps[0].Status = "done"
	snapshot.Steps[2].Status = "active"
	c.apply(snapshot)
	if c.selected != "edit" || !c.inspect {
		t.Fatal("new progress stole the user's selection or inspector")
	}
	view := c.view(80, 18, true)
	if !strings.Contains(view, "Depends on: read") || !strings.Contains(view, "not observed") {
		t.Fatalf("inspector lost real dependency or inferred success: %s", view)
	}
	// Mutating a producer's next snapshot cannot alter accepted display facts.
	snapshot.Steps[1].Dependencies[0] = "mutated"
	if !strings.Contains(c.view(80, 18, true), "Depends on: read") {
		t.Fatal("snapshot retained a mutable producer slice")
	}
	c.key(tea.KeyMsg{Type: tea.KeyEsc})
	if c.inspect || c.selected != "edit" {
		t.Fatal("closing detail lost selection")
	}
	c.apply(CircuitSnapshot{RecipeID: "new", Steps: []CircuitStep{{ID: "start"}}})
	if c.selected != "start" || c.inspect {
		t.Fatal("new recipe retained unrelated selection")
	}
}

func TestV2CircuitHostRetainsDraftAndFocus(t *testing.T) {
	m, _ := newFocusTestModel()
	m.circuit.apply(CircuitSnapshot{RecipeID: "repair", Steps: []CircuitStep{{ID: "read"}, {ID: "edit"}}})
	m.setActiveTab(TabRun)
	m.inputBar.SetValue("unsent request")
	for _, key := range []tea.KeyType{tea.KeyF6, tea.KeyDown, tea.KeyEnter} {
		next, _ := m.Update(tea.KeyMsg{Type: key})
		m = next.(RootModel)
	}
	if !m.circuit.inspect || m.circuit.selected != "edit" || m.inputBar.Value() != "unsent request" {
		t.Fatal("real host keys did not select/inspect while retaining the draft")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(RootModel)
	if m.circuit.inspect || !m.focus.State().InRegion1() {
		t.Fatal("first Escape must close detail and retain circuit focus")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(RootModel)
	if !m.focus.State().InInput() || m.inputBar.Value() != "unsent request" {
		t.Fatal("second Escape must return to unchanged composer")
	}
}

func TestV2CircuitBoundsAndNoInventedStatus(t *testing.T) {
	c := circuitCanvas{}
	snapshot := CircuitSnapshot{RecipeID: "large", Name: "界 e\u0301"}
	for i := range 100 {
		snapshot.Steps = append(snapshot.Steps, CircuitStep{ID: fmt.Sprintf("step-%d", i), Goal: strings.Repeat("wide界 ", 100), Error: "\x1b[2Jinvalid"})
	}
	c.apply(snapshot)
	c.key(tea.KeyMsg{Type: tea.KeyEnd})
	for _, size := range [][2]int{{1, 1}, {39, 9}, {80, 18}, {120, 32}} {
		for _, inspect := range []bool{false, true} {
			c.inspect = inspect
			view := c.view(size[0], size[1], true)
			lines := strings.Split(view, "\n")
			if len(lines) > size[1] {
				t.Fatalf("view exceeded height at %v", size)
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("view exceeded width at %v: %q", size, line)
				}
			}
			if strings.Contains(view, "\x1b") || strings.Contains(view, "completed") {
				t.Fatal("untrusted controls or invented outcomes in circuit")
			}
		}
	}
}
