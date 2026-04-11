package agenttest

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestBuildToolTranscriptPairsCallsAndResults(t *testing.T) {
	base := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)
	artifact := BuildToolTranscript([]core.Event{
		{
			Type:      core.EventToolCall,
			Timestamp: base,
			Message:   "tool file_search invoked",
			Metadata: map[string]any{
				"tool":     "file_search",
				"agent_id": "agent-1",
				"args":     map[string]any{"needle": "euclo"},
			},
		},
		{
			Type:      core.EventToolResult,
			Timestamp: base.Add(2 * time.Second),
			Message:   "tool file_search completed",
			Metadata: map[string]any{
				"tool":        "file_search",
				"success":     true,
				"duration_ms": int64(2000),
			},
		},
	})

	if artifact == nil {
		t.Fatal("expected transcript artifact")
	}
	if len(artifact.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(artifact.Entries))
	}
	entry := artifact.Entries[0]
	if entry.Tool != "file_search" || entry.AgentID != "agent-1" {
		t.Fatalf("unexpected entry: %#v", entry)
	}
	if !entry.Success {
		t.Fatalf("expected success, got %#v", entry)
	}
	if entry.DurationMS != 2000 {
		t.Fatalf("expected 2000ms duration, got %d", entry.DurationMS)
	}
	if entry.CallAt != base || entry.ResultAt != base.Add(2*time.Second) {
		t.Fatalf("unexpected timestamps: %#v", entry)
	}
}

func TestBuildToolTranscriptSkipsUnpairedResults(t *testing.T) {
	artifact := BuildToolTranscript([]core.Event{
		{
			Type:      core.EventToolResult,
			Timestamp: time.Now().UTC(),
			Message:   "tool missing completed",
			Metadata: map[string]any{
				"tool":    "missing",
				"success": false,
			},
		},
	})
	if artifact != nil {
		t.Fatalf("expected nil transcript, got %#v", artifact)
	}
}
