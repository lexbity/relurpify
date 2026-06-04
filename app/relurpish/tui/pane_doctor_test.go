package tui

import (
	"testing"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
)

func TestDoctorStartupLocksWhenReportIsBlocked(t *testing.T) {
	surface := &fakeSurface{name: "none", chat: &fakeChatPane{}}
	factory := &countingFactory{shared: surface}
	m := newRootModel(nil, factory)
	controller, ok := m.baseSurface.(StartupGateController)
	if !ok {
		t.Fatal("expected base surface to expose startup gate controller")
	}
	controller.SetDoctorReport(DoctorReport{})
	m.applyStartupGate()

	if !m.startupLocked {
		t.Fatal("expected startup to be locked for an empty report")
	}
	if got := m.activeAgentName(); got != "none" {
		t.Fatalf("active agent = %q, want none", got)
	}
	if got := m.activeTab; got != TabDoctor {
		t.Fatalf("active tab = %q, want doctor", got)
	}
}

func TestDoctorStartupPromotesToGuestWhenReportIsReady(t *testing.T) {
	guestSurface := &recipeGuestSurface{
		fakeSurface: fakeSurface{name: "guest", chat: &fakeChatPane{}},
	}
	factory := &registryFactory{
		defaultSurface: &baseSurfaceFake{},
		surfaces: map[string]AgentSurface{
			"guest": guestSurface,
		},
	}
	m := newRootModel(nil, factory)
	controller, ok := m.baseSurface.(StartupGateController)
	if !ok {
		t.Fatal("expected base surface to expose startup gate controller")
	}

	controller.SetDoctorReport(DoctorReport{
		WorkspacePresent:      true,
		ConfigExists:          true,
		ManifestExists:        true,
		ModelProfilesExists:   true,
		StarterTemplatesReady: true,
		Dependencies: []runtimesvc.DependencyStatus{
			{Name: "starter-templates", Available: true},
			{Name: "model-profile", Available: true},
		},
	})
	m.applyStartupGate()

	if m.startupLocked {
		t.Fatal("expected startup lock to clear on a healthy report")
	}
	if got := m.activeAgentName(); got != "none" {
		t.Fatalf("active agent = %q, want none", got)
	}
	if got := m.activeTab; got != TabChat {
		t.Fatalf("active tab = %q, want chat", got)
	}
}
