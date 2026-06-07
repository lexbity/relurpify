package event

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type memoryLog struct {
	events    []FrameworkEvent
	snapshots [][]byte
}

func (m *memoryLog) Append(_ context.Context, _ string, events []FrameworkEvent) ([]uint64, error) {
	for i := range events {
		events[i].Seq = uint64(len(m.events) + 1)
		m.events = append(m.events, events[i])
	}
	seqs := make([]uint64, len(events))
	for i := range events {
		seqs[i] = events[i].Seq
	}
	return seqs, nil
}
func (m *memoryLog) Read(_ context.Context, _ string, afterSeq uint64, limit int, _ bool) ([]FrameworkEvent, error) {
	var out []FrameworkEvent
	for _, ev := range m.events {
		if ev.Seq > afterSeq {
			out = append(out, ev)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (m *memoryLog) ReadByType(_ context.Context, _ string, _ string, _ uint64, _ int) ([]FrameworkEvent, error) {
	return nil, nil
}
func (m *memoryLog) LastSeq(_ context.Context, _ string) (uint64, error) {
	return uint64(len(m.events)), nil
}
func (m *memoryLog) TakeSnapshot(_ context.Context, _ string, _ uint64, data []byte) error {
	m.snapshots = append(m.snapshots, append([]byte(nil), data...))
	return nil
}
func (m *memoryLog) LoadSnapshot(_ context.Context, _ string) (uint64, []byte, error) {
	return 0, nil, nil
}
func (m *memoryLog) Close() error { return nil }

type recordingMaterializer struct {
	applied [][]FrameworkEvent
	count   int
}

func (r *recordingMaterializer) Name() string { return "recording" }
func (r *recordingMaterializer) Apply(_ context.Context, events []FrameworkEvent) error {
	r.applied = append(r.applied, append([]FrameworkEvent(nil), events...))
	r.count += len(events)
	return nil
}
func (r *recordingMaterializer) Snapshot(_ context.Context) ([]byte, error) {
	return json.Marshal(map[string]int{"count": r.count})
}
func (r *recordingMaterializer) Restore(_ context.Context, _ []byte) error { return nil }

func TestRunnerRunOnceAppliesAndSnapshots(t *testing.T) {
	log := &memoryLog{}
	_, err := log.Append(context.Background(), "local", []FrameworkEvent{
		{Timestamp: time.Now().UTC(), Type: EventSystemStarted, Partition: "local"},
		{Timestamp: time.Now().UTC(), Type: EventSystemCheckpoint, Partition: "local"},
	})
	require.NoError(t, err)

	mat := &recordingMaterializer{}
	runner := &Runner{Log: log, Materializers: []Materializer{mat}, Partition: "local", SnapshotInterval: 1}

	require.NoError(t, runner.RunOnce(context.Background()))
	require.Len(t, mat.applied, 1)
	require.Len(t, mat.applied[0], 2)
	require.NotEmpty(t, log.snapshots)
}
