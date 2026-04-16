package dispatch

import (
	"context"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type registeredRoutine struct {
	id string
}

func (r registeredRoutine) ID() string { return r.id }

func (r registeredRoutine) Execute(_ context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	if in.State != nil {
		in.State.Set("dispatcher.register_supporting", r.id)
	}
	return []euclotypes.Artifact{{ID: "registered", Kind: euclotypes.ArtifactKindTrace, Summary: "registered", Payload: r.id, ProducerID: r.id, Status: "produced"}}, nil
}

func TestDispatcher_RegistersAllBKCCapabilities(t *testing.T) {
	d := NewDispatcher(agentenv.AgentEnvironment{})

	bkcCaps := []string{
		euclorelurpic.CapabilityBKCCompile,
		euclorelurpic.CapabilityBKCStream,
		euclorelurpic.CapabilityBKCCheckpoint,
		euclorelurpic.CapabilityBKCInvalidate,
	}

	for _, capID := range bkcCaps {
		t.Run(capID, func(t *testing.T) {
			// Check that behavior is registered
			if _, ok := d.behaviors[capID]; !ok {
				t.Errorf("capability %q not registered in dispatcher behaviors", capID)
			}
		})
	}
}

func TestDispatcher_ExecuteRoutine_ReportsMissingRoutine(t *testing.T) {
	d := NewDispatcher(agentenv.AgentEnvironment{})

	_, err := d.ExecuteRoutine(context.Background(), "nonexistent:routine", nil, nil,
		eucloruntime.UnitOfWork{}, agentenv.AgentEnvironment{}, execution.ServiceBundle{})

	if err == nil {
		t.Error("expected error for unregistered routine, got nil")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("expected 'not registered' error, got: %v", err)
	}
}

func TestDispatcher_Execute_ReportsUnavailableBehavior(t *testing.T) {
	d := NewDispatcher(agentenv.AgentEnvironment{})

	input := execution.ExecuteInput{
		Work: eucloruntime.UnitOfWork{
			PrimaryRelurpicCapabilityID: "euclo:nonexistent.behavior",
		},
	}
	_, err := d.Execute(context.Background(), input)

	if err == nil {
		t.Error("expected error for unavailable behavior, got nil")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("expected 'unavailable' error, got: %v", err)
	}
}

func TestDispatcher_BKCCapabilitiesAreBehaviors(t *testing.T) {
	d := NewDispatcher(agentenv.AgentEnvironment{})

	// Verify that BKC capabilities are wrapped as behaviors and can be retrieved
	bkcBehaviors := []string{
		euclorelurpic.CapabilityBKCCompile,
		euclorelurpic.CapabilityBKCStream,
		euclorelurpic.CapabilityBKCCheckpoint,
		euclorelurpic.CapabilityBKCInvalidate,
	}

	for _, capID := range bkcBehaviors {
		behavior, ok := d.behaviors[capID]
		if !ok {
			t.Errorf("BKC capability %q not found in behaviors map", capID)
			continue
		}
		if behavior.ID() != capID {
			t.Errorf("behavior ID mismatch: got %q, want %q", behavior.ID(), capID)
		}
	}
}

func TestDispatcher_RegisterSupportingAddsRoutine(t *testing.T) {
	d := NewDispatcher(agentenv.AgentEnvironment{})
	routine := registeredRoutine{id: "euclo:test.registered"}
	d.RegisterSupporting(routine)

	state := core.NewContext()
	artifacts, err := d.ExecuteRoutine(context.Background(), routine.id, nil, state, eucloruntime.UnitOfWork{}, agentenv.AgentEnvironment{}, execution.ServiceBundle{})
	if err != nil {
		t.Fatalf("ExecuteRoutine: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].ProducerID != routine.id {
		t.Fatalf("unexpected artifacts: %+v", artifacts)
	}
	if got, ok := state.Get("dispatcher.register_supporting"); !ok || got != routine.id {
		t.Fatalf("expected registered routine marker, got %#v", got)
	}
}
