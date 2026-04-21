package plan

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
)

type stubCompatibilityResolver struct{}

func (stubCompatibilityResolver) BackendID() string { return "stub" }
func (stubCompatibilityResolver) Supports(req agentenv.CompatibilitySurfaceRequest) bool {
	return len(req.Files) > 0
}
func (stubCompatibilityResolver) ExtractSurface(_ context.Context, _ agentenv.CompatibilitySurfaceRequest) (agentenv.CompatibilitySurface, bool, error) {
	return agentenv.CompatibilitySurface{
		Functions: []map[string]any{{"name": "Exported"}},
		Metadata:  map[string]any{"source": "resolver"},
	}, true, nil
}

func TestCompatibilitySurfacePlanner_DelegatesToResolver(t *testing.T) {
	planner := NewCompatibilitySurfacePlanner(stubCompatibilityResolver{})
	surface, ok, err := planner.ExtractSurface(context.Background(), agentenv.CompatibilitySurfaceRequest{
		Files: []string{"api.go"},
	})
	if err != nil || !ok {
		t.Fatalf("expected delegated surface, got ok=%v err=%v", ok, err)
	}
	if len(surface.Functions) != 1 || surface.Metadata["backend"] != "stub" {
		t.Fatalf("unexpected surface %#v", surface)
	}
}
