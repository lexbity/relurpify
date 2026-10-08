package orchestrate

import (
	"context"
	"strings"
	"testing"

	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/testsuite/testsupport"
)

func minimalRootGraphDeps() RootGraphDeps {
	return RootGraphDeps{
		DispatchCapabilities: registry.NewRegistry(),
		Paradigm:             &paradigm.Deps{Registry: registry.NewRegistry()},
	}
}

// TestNewRootGraphRequiresHITLBroker pins the fail-closed contract: the graph
// cannot be built without a HITL broker.
func TestNewRootGraphRequiresHITLBroker(t *testing.T) {
	graph, err := NewRootGraph(context.Background(), minimalRootGraphDeps())
	if err == nil {
		t.Fatal("expected an error when the HITL broker is nil")
	}
	if graph != nil {
		t.Fatalf("expected a nil graph, got %#v", graph)
	}
	if !strings.Contains(err.Error(), "HITL broker") {
		t.Fatalf("error %q does not mention the HITL broker", err)
	}
}

// TestNewRootGraphAcceptsHITLBroker proves construction succeeds once a broker
// is supplied.
func TestNewRootGraphAcceptsHITLBroker(t *testing.T) {
	deps := minimalRootGraphDeps()
	deps.HITLBroker = testsupport.NewMockHITLBroker()

	graph, err := NewRootGraph(context.Background(), deps)
	if err != nil {
		t.Fatalf("NewRootGraph failed with a broker: %v", err)
	}
	if graph == nil || graph.Graph() == nil {
		t.Fatal("expected a constructed graph")
	}
}
