package gateway

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/event"
)

type StateSnapshot struct {
	LastSeq         uint64                  `json:"last_seq"`
	ActiveSessions  map[string]SessionState `json:"active_sessions"`
	ChannelActivity map[string]ChannelState `json:"channel_activity"`
	EventTypeCounts map[string]uint64       `json:"event_type_counts"`
}

type SessionState struct {
	Role      string `json:"role,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type ChannelState struct {
	Inbound  uint64 `json:"inbound"`
	Outbound uint64 `json:"outbound"`
}

type StateMaterializer struct {
	mu    sync.RWMutex
	state StateSnapshot
}

func NewStateMaterializer() *StateMaterializer {
	m := &StateMaterializer{}
	m.reset()
	return m
}

func (m *StateMaterializer) Name() string { return "nexus-state" }

func (m *StateMaterializer) Apply(_ context.Context, events []core.FrameworkEvent) error {
	if len(events) == 0 {
		return nil
	}
	// Compute all mutations outside the lock to keep the critical section minimal.
	type sessionOp struct {
		id      string
		state   SessionState
		deleted bool
	}
	type channelOp struct {
		channel  string
		inbound  bool
	}
	var lastSeq uint64
	typeCounts := make(map[string]uint64, len(events))
	var sessionOps []sessionOp
	var channelOps []channelOp

	for _, ev := range events {
		if ev.Seq > lastSeq {
			lastSeq = ev.Seq
		}
		typeCounts[ev.Type]++
		switch ev.Type {
		case core.FrameworkEventSessionCreated:
			sessionOps = append(sessionOps, sessionOp{
				id:    ev.Actor.ID,
				state: SessionState{Role: ev.Actor.Kind, CreatedAt: ev.Timestamp.UTC().Format(timeLayout)},
			})
		case core.FrameworkEventSessionClosed:
			sessionOps = append(sessionOps, sessionOp{id: ev.Actor.ID, deleted: true})
		case core.FrameworkEventMessageInbound:
			if ch := channelFromPayload(ev.Payload); ch != "" {
				channelOps = append(channelOps, channelOp{channel: ch, inbound: true})
			}
		case core.FrameworkEventMessageOutbound:
			if ch := channelFromPayload(ev.Payload); ch != "" {
				channelOps = append(channelOps, channelOp{channel: ch, inbound: false})
			}
		}
	}

	m.mu.Lock()
	if lastSeq > m.state.LastSeq {
		m.state.LastSeq = lastSeq
	}
	for t, n := range typeCounts {
		m.state.EventTypeCounts[t] += n
	}
	for _, op := range sessionOps {
		if op.deleted {
			delete(m.state.ActiveSessions, op.id)
		} else {
			m.state.ActiveSessions[op.id] = op.state
		}
	}
	for _, op := range channelOps {
		s := m.state.ChannelActivity[op.channel]
		if op.inbound {
			s.Inbound++
		} else {
			s.Outbound++
		}
		m.state.ChannelActivity[op.channel] = s
	}
	m.mu.Unlock()
	return nil
}

func (m *StateMaterializer) Snapshot(_ context.Context) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return json.Marshal(m.cloneLocked())
}

func (m *StateMaterializer) Restore(_ context.Context, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reset()
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, &m.state)
}

func (m *StateMaterializer) State() StateSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cloneLocked()
}

func (m *StateMaterializer) reset() {
	m.state = StateSnapshot{
		ActiveSessions:  map[string]SessionState{},
		ChannelActivity: map[string]ChannelState{},
		EventTypeCounts: map[string]uint64{},
	}
}

func (m *StateMaterializer) cloneLocked() StateSnapshot {
	out := StateSnapshot{
		LastSeq:         m.state.LastSeq,
		ActiveSessions:  make(map[string]SessionState, len(m.state.ActiveSessions)),
		ChannelActivity: make(map[string]ChannelState, len(m.state.ChannelActivity)),
		EventTypeCounts: make(map[string]uint64, len(m.state.EventTypeCounts)),
	}
	for key, value := range m.state.ActiveSessions {
		out.ActiveSessions[key] = value
	}
	for key, value := range m.state.ChannelActivity {
		out.ChannelActivity[key] = value
	}
	for key, value := range m.state.EventTypeCounts {
		out.EventTypeCounts[key] = value
	}
	return out
}

func channelFromPayload(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var envelope struct {
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return ""
	}
	return envelope.Channel
}

const timeLayout = "2006-01-02T15:04:05.999999999Z07:00"

var _ event.Materializer = (*StateMaterializer)(nil)
