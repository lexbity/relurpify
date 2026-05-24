package tui

import (
	"testing"
)

func TestInputGateDefaultsToInactive(t *testing.T) {
	g := &InputGate{}
	if g.Active() {
		t.Fatal("expected gate to be inactive by default")
	}
}

func TestInputGateNilSafe(t *testing.T) {
	var g *InputGate
	g.SetActive(true)
	if g.Active() {
		t.Fatal("expected nil-safe Active to return false")
	}
}

func TestInputGateSetActive(t *testing.T) {
	g := &InputGate{}
	g.SetActive(true)
	if !g.Active() {
		t.Fatal("expected gate to be active after SetActive(true)")
	}
	g.SetActive(false)
	if g.Active() {
		t.Fatal("expected gate to be inactive after SetActive(false)")
	}
}

func TestInputGateTogglesBackAndForth(t *testing.T) {
	g := &InputGate{}
	if g.Active() {
		t.Fatal("expected initial inactive")
	}
	g.SetActive(true)
	if !g.Active() {
		t.Fatal("expected active after set")
	}
	g.SetActive(true)
	if !g.Active() {
		t.Fatal("expected still active after second set")
	}
	g.SetActive(false)
	if g.Active() {
		t.Fatal("expected inactive after clear")
	}
	g.SetActive(false)
	if g.Active() {
		t.Fatal("expected still inactive after second clear")
	}
}
