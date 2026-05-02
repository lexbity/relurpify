package orchestrate

import (
	"context"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type dryRunCountingHandler struct {
	id           string
	invocations  int
	mu           sync.Mutex
	availability core.AvailabilitySpec
}

func (h *dryRunCountingHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            h.id,
		Name:          h.id,
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
		Availability:  h.availability,
	}
}

func (h *dryRunCountingHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*contracts.CapabilityExecutionResult, error) {
	h.mu.Lock()
	h.invocations++
	h.mu.Unlock()
	return &contracts.CapabilityExecutionResult{Success: true, Data: map[string]interface{}{"ok": true}}, nil
}

func (h *dryRunCountingHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.invocations
}

func TestDryRun_ReturnsReport_NoExecution(t *testing.T) {
	reg := capability.NewCapabilityRegistry()
	handler := &dryRunCountingHandler{
		id:           "euclo:cap.ast_query",
		availability: core.AvailabilitySpec{Available: true},
	}
	if err := reg.RegisterInvocableCapability(handler); err != nil {
		t.Fatalf("register handler: %v", err)
	}

	report, err := DryRun(context.Background(), contextdata.NewEnvelope("task-1", "session-1"), RouteRequest{CapabilityID: handler.id, DryRun: true}, reg, nil)
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}
	if report == nil {
		t.Fatal("expected dry-run report")
	}
	if got := handler.count(); got != 0 {
		t.Fatalf("expected no execution, got %d invocations", got)
	}
}

func TestDryRun_IncludesAllCandidates(t *testing.T) {
	reg := capability.NewCapabilityRegistry()
	one := &dryRunCountingHandler{id: "euclo:cap.ast_query", availability: core.AvailabilitySpec{Available: true}}
	two := &dryRunCountingHandler{id: "euclo:cap.symbol_trace", availability: core.AvailabilitySpec{Available: true}}
	if err := reg.RegisterInvocableCapability(one); err != nil {
		t.Fatalf("register one: %v", err)
	}
	if err := reg.RegisterInvocableCapability(two); err != nil {
		t.Fatalf("register two: %v", err)
	}

	report, err := DryRun(context.Background(), contextdata.NewEnvelope("task-1", "session-1"), RouteRequest{FamilyID: "query", DryRun: true}, reg, nil)
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}
	if got := len(report.Candidates); got != 2 {
		t.Fatalf("expected 2 candidates, got %d", got)
	}
}

func TestDryRun_PolicyDenied_InCandidateList(t *testing.T) {
	reg := capability.NewCapabilityRegistry()
	visible := &dryRunCountingHandler{id: "euclo:cap.ast_query", availability: core.AvailabilitySpec{Available: true}}
	hidden := &dryRunCountingHandler{id: "euclo:cap.code_review", availability: core.AvailabilitySpec{Available: true}}
	if err := reg.RegisterInvocableCapability(visible); err != nil {
		t.Fatalf("register visible: %v", err)
	}
	if err := reg.RegisterInvocableCapability(hidden); err != nil {
		t.Fatalf("register hidden: %v", err)
	}
	reg.AddExposurePolicies([]core.CapabilityExposurePolicy{{
		Selector: core.CapabilitySelector{ID: hidden.id},
		Access:   core.CapabilityExposureHidden,
	}})

	report, err := DryRun(context.Background(), contextdata.NewEnvelope("task-1", "session-1"), RouteRequest{DryRun: true}, reg, nil)
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}
	var found bool
	for _, candidate := range report.Candidates {
		if candidate.RouteID == RouteID(hidden.id) {
			found = true
			if candidate.Availability != RouteUnavailablePolicyDenied {
				t.Fatalf("expected hidden candidate to be policy denied, got %q", candidate.Availability)
			}
			if !candidate.Suppressed {
				t.Fatalf("expected hidden candidate to be suppressed")
			}
		}
	}
	if !found {
		t.Fatalf("expected hidden candidate in report: %+v", report.Candidates)
	}
}

func TestDryRun_SamePreflightAsLiveExecution(t *testing.T) {
	reg := capability.NewCapabilityRegistry()
	primary := &dryRunCountingHandler{id: "euclo:cap.ast_query", availability: core.AvailabilitySpec{Available: true}}
	if err := reg.RegisterInvocableCapability(primary); err != nil {
		t.Fatalf("register primary: %v", err)
	}

	env := contextdata.NewEnvelope("task-1", "session-1")
	req := RouteRequest{CapabilityID: primary.id}

	live, err := Dispatch(context.Background(), env, req, reg, nil)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	report, err := DryRun(context.Background(), contextdata.NewEnvelope("task-1", "session-1"), req, reg, nil)
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}
	if live.RouteID != string(report.SelectedRoute) {
		t.Fatalf("expected live and dry-run to select same route, live=%q dry=%q", live.RouteID, report.SelectedRoute)
	}
}

func TestDryRun_EmitsDryRunEvent(t *testing.T) {
	reg := capability.NewCapabilityRegistry()
	primary := &dryRunCountingHandler{id: "euclo:cap.ast_query", availability: core.AvailabilitySpec{Available: true}}
	if err := reg.RegisterInvocableCapability(primary); err != nil {
		t.Fatalf("register primary: %v", err)
	}

	sink := &telemetrySink{}
	ctx := core.WithTelemetry(context.Background(), sink)
	if _, err := DryRun(ctx, contextdata.NewEnvelope("task-1", "session-1"), RouteRequest{CapabilityID: primary.id, DryRun: true}, reg, nil); err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}

	found := false
	for _, event := range sink.snapshot() {
		if event.Type == core.EventType("euclo.route.dry_run") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected euclo.route.dry_run event")
	}
}
