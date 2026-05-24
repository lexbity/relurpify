package euclotui

import (
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/relurpifyenvtui"
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
)

func TestEucloSurfaceFactoryResolvesGuestAndBaseSurfaces(t *testing.T) {
	factory := NewSurfaceFactory()
	agents := factory.AvailableAgents()
	if len(agents) != 1 || agents[0] != "euclo" {
		t.Fatalf("available agents = %#v, want [euclo]", agents)
	}

	guest := factory.Resolve("euclo")
	if guest == nil {
		t.Fatal("expected euclo surface")
	}
	if got := guest.Name(); got != "euclo" {
		t.Fatalf("guest name = %q, want euclo", got)
	}

	base := factory.Resolve("none")
	if base == nil {
		t.Fatal("expected base surface")
	}
	if got := base.Name(); got != "none" {
		t.Fatalf("base name = %q, want none", got)
	}
}

func TestEucloSurfaceCreatesLibrarySurface(t *testing.T) {
	surface, ok := NewSurface().(*EucloSurface)
	if !ok {
		t.Fatalf("surface type = %T, want *EucloSurface", NewSurface())
	}

	lib := surface.NewLibrary(nil, &tui.AgentContext{}, &tui.Session{})
	if lib == nil {
		t.Fatal("expected library surface")
	}
	if got := lib.View(); got == "" {
		t.Fatal("expected non-empty library view")
	}
}

func TestBaseSurfaceFallbackLivesInDedicatedPackage(t *testing.T) {
	surface := relurpifyenvtui.NewSurface()
	if surface == nil {
		t.Fatal("expected dedicated base-framework surface")
	}
	if got := surface.Name(); got != "none" {
		t.Fatalf("base surface name = %q, want none", got)
	}
}
