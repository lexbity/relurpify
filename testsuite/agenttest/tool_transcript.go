package agenttest

import (
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

type ToolTranscriptArtifact struct {
	Entries []ToolTranscriptEntry `json:"entries"`
}

type ToolTranscriptEntry struct {
	Index          int            `json:"index"`
	Tool           string         `json:"tool,omitempty"`
	AgentID        string         `json:"agent_id,omitempty"`
	CallAt         time.Time      `json:"call_at,omitempty"`
	CallMessage    string         `json:"call_message,omitempty"`
	CallMetadata   map[string]any `json:"call_metadata,omitempty"`
	ResultAt       time.Time      `json:"result_at,omitempty"`
	ResultMessage  string         `json:"result_message,omitempty"`
	ResultMetadata map[string]any `json:"result_metadata,omitempty"`
	DurationMS     int64          `json:"duration_ms,omitempty"`
	Success        bool           `json:"success,omitempty"`
	Error          string         `json:"error,omitempty"`
}

func BuildToolTranscript(events []core.Event) *ToolTranscriptArtifact {
	if len(events) == 0 {
		return nil
	}
	var (
		entries    []ToolTranscriptEntry
		pendingIx  = map[string][]int{}
		pendingCnt int
	)
	for _, ev := range events {
		switch ev.Type {
		case core.EventToolCall:
			entry := ToolTranscriptEntry{
				Index:        pendingCnt,
				Tool:         stringValue(ev.Metadata["tool"]),
				AgentID:      stringValue(ev.Metadata["agent_id"]),
				CallAt:       ev.Timestamp,
				CallMessage:  ev.Message,
				CallMetadata: cloneAnyMap(ev.Metadata),
			}
			entries = append(entries, entry)
			pendingIx[entry.Tool] = append(pendingIx[entry.Tool], len(entries)-1)
			pendingCnt++
		case core.EventToolResult:
			tool := stringValue(ev.Metadata["tool"])
			if tool == "" {
				continue
			}
			indexes := pendingIx[tool]
			if len(indexes) == 0 {
				continue
			}
			entryIdx := indexes[0]
			pendingIx[tool] = indexes[1:]
			entries[entryIdx].ResultAt = ev.Timestamp
			entries[entryIdx].ResultMessage = ev.Message
			entries[entryIdx].ResultMetadata = cloneAnyMap(ev.Metadata)
			entries[entryIdx].DurationMS = int64Value(ev.Metadata["duration_ms"])
			if success, ok := ev.Metadata["success"].(bool); ok {
				entries[entryIdx].Success = success
			}
			entries[entryIdx].Error = firstNonEmpty(
				stringValue(ev.Metadata["error"]),
				stringValue(ev.Metadata["tool_error"]),
			)
		}
	}
	if len(entries) == 0 {
		return nil
	}
	return &ToolTranscriptArtifact{Entries: entries}
}

func cloneAnyMap(raw map[string]interface{}) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		out[k] = v
	}
	return out
}

func stringValue(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

func int64Value(raw any) int64 {
	switch v := raw.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	default:
		return 0
	}
}
