package fmp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

type ChunkTransferManager interface {
	Open(ctx context.Context, manifest core.ContextManifest, sealed core.SealedContext, now time.Time) (*ChunkTransferSession, error)
	Read(ctx context.Context, transferID string, maxChunks int, now time.Time) ([]ChunkFrame, *ChunkFlowControl, error)
	Ack(ctx context.Context, transferID string, ack ChunkAck, now time.Time) (*ChunkFlowControl, error)
	Cancel(ctx context.Context, transferID string, reason string, now time.Time) error
}

type ChunkTransferSession struct {
	TransferID   string            `json:"transfer_id" yaml:"transfer_id"`
	ManifestRef  string            `json:"manifest_ref" yaml:"manifest_ref"`
	TransferMode core.TransferMode `json:"transfer_mode" yaml:"transfer_mode"`
	TotalChunks  int               `json:"total_chunks" yaml:"total_chunks"`
	WindowSize   int               `json:"window_size,omitempty" yaml:"window_size,omitempty"`
	ExpiresAt    time.Time         `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

type ChunkFrame struct {
	TransferID string `json:"transfer_id" yaml:"transfer_id"`
	Index      int    `json:"index" yaml:"index"`
	Final      bool   `json:"final,omitempty" yaml:"final,omitempty"`
	Payload    []byte `json:"payload,omitempty" yaml:"payload,omitempty"`
}

type ChunkFlowControl struct {
	TransferID   string `json:"transfer_id" yaml:"transfer_id"`
	NextChunk    int    `json:"next_chunk" yaml:"next_chunk"`
	Remaining    int    `json:"remaining" yaml:"remaining"`
	WindowSize   int    `json:"window_size,omitempty" yaml:"window_size,omitempty"`
	Completed    bool   `json:"completed,omitempty" yaml:"completed,omitempty"`
	Cancelled    bool   `json:"cancelled,omitempty" yaml:"cancelled,omitempty"`
	Backpressure bool   `json:"backpressure,omitempty" yaml:"backpressure,omitempty"`
}

type ChunkAck struct {
	TransferID string `json:"transfer_id" yaml:"transfer_id"`
	AckedIndex int    `json:"acked_index" yaml:"acked_index"`
	WindowSize int    `json:"window_size,omitempty" yaml:"window_size,omitempty"`
}

type InMemoryChunkTransferManager struct {
	DefaultWindow int
	ChunkTTL      time.Duration

	mu       sync.Mutex
	sessions map[string]*chunkTransferState
}

type chunkTransferState struct {
	session   ChunkTransferSession
	chunks    [][]byte
	nextChunk int
	lastAck   int
	cancelled bool
}

func (m *InMemoryChunkTransferManager) Open(_ context.Context, manifest core.ContextManifest, sealed core.SealedContext, now time.Time) (*ChunkTransferSession, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	if err := sealed.Validate(); err != nil {
		return nil, err
	}
	if manifest.TransferMode != core.TransferModeChunked {
		return nil, fmt.Errorf("chunk transfer requires manifest transfer_mode=chunked")
	}
	if len(sealed.CiphertextChunks) == 0 {
		return nil, fmt.Errorf("chunk transfer requires inline ciphertext chunks")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	window := m.DefaultWindow
	if window <= 0 {
		window = 2
	}
	ttl := m.ChunkTTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	session := ChunkTransferSession{
		TransferID:   manifest.ContextID + ":chunk",
		ManifestRef:  manifest.ContextID,
		TransferMode: manifest.TransferMode,
		TotalChunks:  len(sealed.CiphertextChunks),
		WindowSize:   window,
		ExpiresAt:    now.Add(ttl),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions == nil {
		m.sessions = map[string]*chunkTransferState{}
	}
	chunks := make([][]byte, len(sealed.CiphertextChunks))
	for i := range sealed.CiphertextChunks {
		chunks[i] = append([]byte(nil), sealed.CiphertextChunks[i]...)
	}
	m.sessions[session.TransferID] = &chunkTransferState{
		session: session,
		chunks:  chunks,
		lastAck: -1,
	}
	copy := session
	return &copy, nil
}

func (m *InMemoryChunkTransferManager) Read(_ context.Context, transferID string, maxChunks int, now time.Time) ([]ChunkFrame, *ChunkFlowControl, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.activeStateLocked(transferID, now)
	if err != nil {
		return nil, nil, err
	}
	if state.cancelled {
		return nil, nil, fmt.Errorf("chunk transfer %s cancelled", transferID)
	}
	if maxChunks <= 0 {
		maxChunks = 1
	}
	limit := minInt(maxChunks, state.session.WindowSize)
	if limit <= 0 {
		limit = 1
	}
	if state.nextChunk-state.lastAck-1 >= state.session.WindowSize {
		return nil, &ChunkFlowControl{
			TransferID:   transferID,
			NextChunk:    state.nextChunk,
			Remaining:    len(state.chunks) - state.nextChunk,
			WindowSize:   state.session.WindowSize,
			Backpressure: true,
		}, nil
	}
	out := make([]ChunkFrame, 0, limit)
	for i := 0; i < limit && state.nextChunk < len(state.chunks); i++ {
		index := state.nextChunk
		out = append(out, ChunkFrame{
			TransferID: transferID,
			Index:      index,
			Final:      index == len(state.chunks)-1,
			Payload:    append([]byte(nil), state.chunks[index]...),
		})
		state.nextChunk++
	}
	control := &ChunkFlowControl{
		TransferID: transferID,
		NextChunk:  state.nextChunk,
		Remaining:  len(state.chunks) - state.nextChunk,
		WindowSize: state.session.WindowSize,
		Completed:  state.nextChunk >= len(state.chunks) && state.lastAck >= len(state.chunks)-1,
	}
	return out, control, nil
}

func (m *InMemoryChunkTransferManager) Ack(_ context.Context, transferID string, ack ChunkAck, now time.Time) (*ChunkFlowControl, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.activeStateLocked(transferID, now)
	if err != nil {
		return nil, err
	}
	if ack.AckedIndex < state.lastAck {
		return nil, fmt.Errorf("acked index %d regressed for transfer %s", ack.AckedIndex, transferID)
	}
	if ack.AckedIndex >= len(state.chunks) {
		return nil, fmt.Errorf("acked index %d out of range for transfer %s", ack.AckedIndex, transferID)
	}
	state.lastAck = ack.AckedIndex
	if ack.WindowSize > 0 {
		state.session.WindowSize = ack.WindowSize
	}
	return &ChunkFlowControl{
		TransferID: transferID,
		NextChunk:  state.nextChunk,
		Remaining:  len(state.chunks) - state.nextChunk,
		WindowSize: state.session.WindowSize,
		Completed:  state.nextChunk >= len(state.chunks) && state.lastAck >= len(state.chunks)-1,
		Cancelled:  state.cancelled,
	}, nil
}

func (m *InMemoryChunkTransferManager) Cancel(_ context.Context, transferID string, reason string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.activeStateLocked(transferID, now)
	if err != nil {
		return err
	}
	state.cancelled = true
	if reason == "" {
		reason = "cancelled"
	}
	return nil
}

func (m *InMemoryChunkTransferManager) activeStateLocked(transferID string, now time.Time) (*chunkTransferState, error) {
	if transferID == "" {
		return nil, fmt.Errorf("transfer id required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	for id, state := range m.sessions {
		if now.After(state.session.ExpiresAt) {
			delete(m.sessions, id)
		}
	}
	state, ok := m.sessions[transferID]
	if !ok {
		return nil, fmt.Errorf("chunk transfer %s not found", transferID)
	}
	return state, nil
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
