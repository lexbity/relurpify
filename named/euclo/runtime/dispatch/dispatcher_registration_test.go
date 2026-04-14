package dispatch

import (
	"context"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

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
