package euclo

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestAgentWiringHelpersCoverThinBoundaries(t *testing.T) {
	agent := &Agent{}

	if got := agent.Capabilities(); len(got) != 5 {
		t.Fatalf("unexpected capabilities: %#v", got)
	}
	if got := agent.workflowStore(); got != nil {
		t.Fatalf("expected nil workflow store, got %#v", got)
	}

	_ = agent.phaseService()
	_ = agent.archaeologyService()
	_ = agent.learningService()
	_ = agent.verificationService()
	_ = agent.planService()
	_ = agent.tensionService()
	_ = agent.executionService()
	_ = agent.executionHandoffRecorder()
	_ = agent.preflightCoordinator()
	_ = agent.liveMutationCoordinator()
	_ = agent.createInteractionRegistry()
	if agent.ConfigTelemetry() != nil {
		t.Fatal("expected nil telemetry on zero-value agent")
	}

	if agent.hydratePersistedArtifacts(context.Background(), nil, nil) {
		t.Fatal("expected hydration to be skipped with nil state")
	}
	if got := gitCheckpoint(context.Background(), nil, nil); got != "" {
		t.Fatalf("expected empty git checkpoint, got %q", got)
	}
	if got := gitCheckpoint(context.Background(), &core.Task{Context: map[string]any{"workspace": " "}}, nil); got != "" {
		t.Fatalf("expected empty git checkpoint for blank workspace, got %q", got)
	}
}
