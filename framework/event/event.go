package event

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// Log is the append-only framework event log.
type Log interface {
	Append(ctx context.Context, partition string, events []core.FrameworkEvent) ([]uint64, error)
	Read(ctx context.Context, partition string, afterSeq uint64, limit int, follow bool) ([]core.FrameworkEvent, error)
	ReadByType(ctx context.Context, partition string, typePrefix string, afterSeq uint64, limit int) ([]core.FrameworkEvent, error)
	LastSeq(ctx context.Context, partition string) (uint64, error)
	TakeSnapshot(ctx context.Context, partition string, seq uint64, data []byte) error
	LoadSnapshot(ctx context.Context, partition string) (uint64, []byte, error)
	Close() error
}

// Materializer consumes events and maintains a derived state view.
type Materializer interface {
	Name() string
	Apply(ctx context.Context, events []core.FrameworkEvent) error
	Snapshot(ctx context.Context) ([]byte, error)
	Restore(ctx context.Context, data []byte) error
}

// Runner tails a log and applies events to materializers.
type Runner struct {
	Log              Log
	Materializers    []Materializer
	Partition        string
	SnapshotInterval int
	lastSnapshotSeq  uint64
}

func (r *Runner) Run(ctx context.Context) error {
	if r == nil || r.Log == nil {
		return nil
	}
	after := uint64(0)
	for {
		events, err := r.Log.Read(ctx, partitionOrDefault(r.Partition), after, 256, true)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		if err := r.applyBatch(ctx, events); err != nil {
			return err
		}
		after = events[len(events)-1].Seq
	}
}

func (r *Runner) RunOnce(ctx context.Context) error {
	if r == nil || r.Log == nil {
		return nil
	}
	events, err := r.Log.Read(ctx, partitionOrDefault(r.Partition), 0, 0, false)
	if err != nil {
		return err
	}
	return r.applyBatch(ctx, events)
}

// RestoreAndRunOnce loads the latest snapshot for the runner's partition, applies
// all events since the snapshot, and saves a new snapshot. This is the standard
// startup path for materializers that need to catch up before going live.
func (r *Runner) RestoreAndRunOnce(ctx context.Context) error {
	if r == nil || r.Log == nil {
		return nil
	}
	partition := partitionOrDefault(r.Partition)
	seq, data, err := r.Log.LoadSnapshot(ctx, partition)
	if err != nil {
		return err
	}
	if len(data) > 0 {
		for _, m := range r.Materializers {
			if err := m.Restore(ctx, data); err != nil {
				return err
			}
		}
	}
	events, err := r.Log.Read(ctx, partition, seq, 0, false)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	if err := r.applyBatch(ctx, events); err != nil {
		return err
	}
	lastSeq := events[len(events)-1].Seq
	for _, m := range r.Materializers {
		snapshot, err := m.Snapshot(ctx)
		if err != nil {
			return err
		}
		if err := r.Log.TakeSnapshot(ctx, partition, lastSeq, snapshot); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) applyBatch(ctx context.Context, events []core.FrameworkEvent) error {
	if len(events) == 0 {
		return nil
	}
	for _, m := range r.Materializers {
		if err := m.Apply(ctx, events); err != nil {
			return err
		}
	}
	if r.SnapshotInterval <= 0 {
		r.SnapshotInterval = 10000
	}
	lastSeq := events[len(events)-1].Seq
	if lastSeq-r.lastSnapshotSeq < uint64(r.SnapshotInterval) {
		return nil
	}
	for _, m := range r.Materializers {
		data, err := m.Snapshot(ctx)
		if err != nil {
			return err
		}
		if err := r.Log.TakeSnapshot(ctx, partitionOrDefault(r.Partition), lastSeq, data); err != nil {
			return err
		}
	}
	r.lastSnapshotSeq = lastSeq
	return nil
}

func partitionOrDefault(partition string) string {
	if partition == "" {
		return "local"
	}
	return partition
}
