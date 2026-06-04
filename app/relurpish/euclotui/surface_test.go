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

	reg := tui.NewTabRegistry()
	RegisterEucloTabs(reg)
	tabs := reg.TabsForAgent("euclo")
	if len(tabs) != 3 || tabs[0].ID != tui.TabChat || tabs[1].ID != tui.TabDiff || tabs[2].ID != TabRecipe {
		t.Fatalf("euclo tabs = %#v, want [chat diff]", tabs)
	}

	base := factory.Resolve("none")
	if base == nil {
		t.Fatal("expected base surface")
	}
	if got := base.Name(); got != "none" {
		t.Fatalf("base name = %q, want none", got)
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
