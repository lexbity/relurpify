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

type registeredInvocable struct {
	id string
}

func (r registeredInvocable) ID() string      { return r.id }
func (r registeredInvocable) IsPrimary() bool { return false }
func (r registeredInvocable) Invoke(_ context.Context, in execution.InvokeInput) (*core.Result, error) {
	if in.State != nil {
		in.State.Set("dispatcher.register_supporting", r.id)
	}
	return &core.Result{Success: true, Data: map[string]any{"artifacts": []euclotypes.Artifact{{ID: "registered", Kind: euclotypes.ArtifactKindTrace, Summary: "registered", Payload: r.id, ProducerID: r.id, Status: "produced"}}}}, nil
}

func TestDispatcher_RegistersAllBKCCapabilities(t *testing.T) {
	d := NewDispatcher(agentenv.AgentEnvironment{})

	for _, capID := range []string{
		euclorelurpic.CapabilityBKCCompile,
		euclorelurpic.CapabilityBKCStream,
		euclorelurpic.CapabilityBKCCheckpoint,
		euclorelurpic.CapabilityBKCInvalidate,
	} {
		if _, ok := d.invocables[capID]; !ok {
			t.Fatalf("capability %q not registered in dispatcher", capID)
		}
	}
}

func TestDispatcher_ExecuteRoutine_ReportsMissingRoutine(t *testing.T) {
	d := NewDispatcher(agentenv.AgentEnvironment{})

	_, err := d.ExecuteRoutine(context.Background(), "nonexistent:routine", nil, nil,
		eucloruntime.UnitOfWork{}, agentenv.AgentEnvironment{}, execution.ServiceBundle{})

	if err == nil {
		t.Fatal("expected error for unregistered routine, got nil")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("expected 'not registered' error, got: %v", err)
	}
}

func TestDispatcher_Execute_ReportsUnavailableInvocable(t *testing.T) {
	d := NewDispatcher(agentenv.AgentEnvironment{})

	input := execution.ExecuteInput{
		Work: eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{PrimaryRelurpicCapabilityID: "euclo:nonexistent.behavior"}},
	}
	_, err := d.Execute(context.Background(), input)

	if err == nil {
		t.Error("expected error for unavailable behavior, got nil")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("expected 'unavailable' error, got: %v", err)
	}
}

func TestDispatcher_RegisterSupportingAddsRoutine(t *testing.T) {
	d := NewDispatcher(agentenv.AgentEnvironment{})
	routine := registeredInvocable{id: "euclo:test.registered"}
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
