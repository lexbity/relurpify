package gateway

import (
	"context"
	"encoding/json"

	"codeburg.org/lexbit/relurpify/framework/core"
)

type replayEventFrame struct {
	Type   string              `json:"type"`
	Replay bool                `json:"replay"`
	Event  core.FrameworkEvent `json:"event"`
}

type replayCompleteFrame struct {
	Type       string `json:"type"`
	Replay     bool   `json:"replay"`
	LastSeq    uint64 `json:"last_seq"`
	EventCount int    `json:"event_count"`
}

func (s *Server) replayFrames(ctx context.Context, principal ConnectionPrincipal, lastSeenSeq uint64) ([]any, error) {
	if s == nil || s.Log == nil {
		return []any{replayCompleteFrame{Type: "replay_complete", Replay: true, LastSeq: 0, EventCount: 0}}, nil
	}
	events, err := s.Log.Read(ctx, s.partition(), lastSeenSeq, 256, false)
	if err != nil {
		return nil, err
	}
	frames := make([]any, 0, len(events)+1)
	lastSeq := lastSeenSeq
	delivered := 0
	for _, ev := range events {
		allowed, err := s.canDeliverEvent(ctx, principal, ev)
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}
		frames = append(frames, replayEventFrame{
			Type:   "event",
			Replay: true,
			Event:  ev,
		})
		lastSeq = ev.Seq
		delivered++
	}
	frames = append(frames, replayCompleteFrame{
		Type:       "replay_complete",
		Replay:     true,
		LastSeq:    lastSeq,
		EventCount: delivered,
	})
	return frames, nil
}

func mustReplayJSON(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
