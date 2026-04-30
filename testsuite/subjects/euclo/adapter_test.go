package euclo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

func TestCollectPerformancePhases(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("euclo.interaction_state", map[string]any{
		"phases_executed": []string{"plan", "review"},
	}, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.profile_phase_records", []any{
		map[string]any{"phase": "plan"},
	}, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.interaction_records", []any{
		map[string]any{"phase": "review", "duration": "25ms"},
	}, contextdata.MemoryClassTask)

	got := CollectPerformancePhases(env)
	if len(got) != 2 || got[0] != "plan" || got[1] != "review" {
		t.Fatalf("unexpected phases: %#v", got)
	}
	if ms := PhaseDurationMS(env, "review"); ms != 25 {
		t.Fatalf("expected 25ms duration, got %d", ms)
	}
}

func TestWriteInteractionTape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "interaction.tape.jsonl")
	err := WriteInteractionTape(path, map[string]any{
		"euclo.interaction_records": []any{
			map[string]any{"phase": "review", "kind": "proposal"},
			map[string]any{"phase": "review", "kind": "question"},
		},
	})
	if err != nil {
		t.Fatalf("WriteInteractionTape: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 tape lines, got %d", len(lines))
	}
}
