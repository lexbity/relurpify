package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// varBlock keeps gosec G101 quiet about the golden-fingerprint bytes below.
var (
	_ = `{"timestamp":"2026-10-08T12:00:00.123456789Z","agent_id":"agent-ws-euclo","action":"exec:binary:docker","type":"executable","permission":"docker","result":"granted","metadata":{"args":["docker","version"]}}`
	_ = `{"sequence":418,"record":{"timestamp":"2026-10-08T12:00:00.123456789Z","agent_id":"agent-ws-euclo","action":"exec:binary:docker","type":"executable","permission":"docker","result":"granted","metadata":{"args":["docker","version"]}}}`
)

// newChainLogger builds a chain logger with an injectable clock.
func newChainLogger(t *testing.T, now *time.Time, opts FileChainOptions) *FileChainAuditLogger {
	t.Helper()
	if opts.Now == nil {
		opts.Now = func() time.Time { return *now }
	}
	l, err := NewFileChainAuditLogger(t.TempDir(), opts)
	require.NoError(t, err)
	return l
}

func goldenRecord() AuditRecord {
	return AuditRecord{
		Timestamp:  time.Date(2026, 10, 8, 12, 0, 0, 123456789, time.UTC),
		AgentID:    "agent-ws-euclo",
		Action:     "exec:binary:docker",
		Type:       "executable",
		Permission: "docker",
		Result:     "granted",
		Metadata:   map[string]any{"args": []string{"docker", "version"}},
	}
}

// TestCanonicalJSON_MustRemainStable pins the canonical JSON bytes of one
// record. The on-disk chain hashes these exact bytes; any change to the
// AuditRecord shape or Marshal behavior breaks every existing chain and MUST
// bump AuditChainFormatV1. This test enforces the conscious decision (SBH-1
// D-10 §5.3).
func TestCanonicalJSON_MustRemainStable(t *testing.T) {
	got := string(canonicalJSON(goldenRecord()))
	want := `{"timestamp":"2026-10-08T12:00:00.123456789Z","agent_id":"agent-ws-euclo","action":"exec:binary:docker","type":"executable","permission":"docker","result":"granted","metadata":{"args":["docker","version"]}}`
	require.Equal(t, want, got)
	require.Equal(t, AuditChainFormatV1, "audit_chain_v1", "format constant must describe the pinned shape")
}

func TestChainRecordHash_Deterministic(t *testing.T) {
	a := chainRecordHash(auditChainGenesisHash, goldenRecord())
	b := chainRecordHash(auditChainGenesisHash, goldenRecord())
	require.Equal(t, a, b)
	require.Len(t, a, 64)
	require.NotEqual(t, a, chainRecordHash("some-other-prev", goldenRecord()))
}

func TestChainLogger_AppendAndVerifyAcrossRotation(t *testing.T) {
	now := time.Now()
	l := newChainLogger(t, &now, FileChainOptions{BatchFlush: 10 * time.Millisecond})
	ctx := context.Background()

	const total = 10_000
	for i := 0; i < total; i++ {
		if i == total/2 {
			// Roll the day: the second half lands in a new chain file.
			now = now.Add(24 * time.Hour)
		}
		require.NoError(t, l.Log(ctx, AuditRecord{
			AgentID: "agent-a",
			Action:  "exec:binary:ssh",
			Type:    "executable",
			Result:  "granted",
		}))
	}

	v, err := l.VerifyChain(ctx, AuditChainFilter{})
	require.NoError(t, err)
	require.True(t, v.Verified, "chain must verify: %s", v.Failure)
	require.Equal(t, int64(total), v.LastSequence)
	require.Equal(t, v.LastSequence, v.LastSequence)

	files, err := chainFilesIn(l.dir)
	require.NoError(t, err)
	require.Len(t, files, 2, "expected one file per day")

	// Query sees every committed record.
	records, err := l.Query(ctx, AuditQuery{AgentID: "agent-a"})
	require.NoError(t, err)
	require.Len(t, records, total)
	require.NoError(t, l.Close())
}

func TestChainLogger_ByteFlipInMiddleFileFailsVerify(t *testing.T) {
	now := time.Now()
	l := newChainLogger(t, &now, FileChainOptions{})
	ctx := context.Background()
	for i := 0; i < 200; i++ {
		if i == 100 {
			now = now.Add(24 * time.Hour)
		}
		require.NoError(t, l.Log(ctx, AuditRecord{AgentID: "agent-a", Action: "exec:binary:ssh", Type: "executable", Result: "granted"}))
	}
	require.NoError(t, l.Close())

	files, err := chainFilesIn(l.dir)
	require.NoError(t, err)
	require.Len(t, files, 2)

	// Flip one byte in the second file (mid-chain from the head's point of
	// view).
	data, err := os.ReadFile(files[1])
	require.NoError(t, err)
	data[len(data)/2] ^= 0xff
	require.NoError(t, os.WriteFile(files[1], data, 0o640))

	v, err := l.VerifyChain(ctx, AuditChainFilter{})
	require.NoError(t, err)
	require.False(t, v.Verified, "byte flip must break verification")
	require.NotEmpty(t, v.Failure)

	// The full-replay doctor probe also reports broken.
	probe := ProbeChainDir(l.dir)
	require.True(t, probe.Broken)
	require.True(t, probe.Present)
}

func TestChainLogger_TruncatedTailFailsVerify(t *testing.T) {
	now := time.Now()
	l := newChainLogger(t, &now, FileChainOptions{})
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		require.NoError(t, l.Log(ctx, AuditRecord{AgentID: "agent-a", Action: "exec:binary:ssh", Type: "executable", Result: "granted"}))
	}
	require.NoError(t, l.Close())

	files, err := chainFilesIn(l.dir)
	require.NoError(t, err)
	require.Len(t, files, 1)

	data, err := os.ReadFile(files[0])
	require.NoError(t, err)
	// Chop half the file's tail.
	require.NoError(t, os.WriteFile(files[0], data[:len(data)/2], 0o640))

	v, err := l.VerifyChain(ctx, AuditChainFilter{})
	require.NoError(t, err)
	require.False(t, v.Verified, "truncated tail must break verification")
	require.True(t, ProbeChainDir(l.dir).Broken)
}

func TestChainLogger_DeleteHeadRebuildsByFullScan(t *testing.T) {
	dir := t.TempDir()
	first, err := NewFileChainAuditLogger(dir, FileChainOptions{BatchFlush: 5 * time.Millisecond})
	require.NoError(t, err)
	for i := 0; i < 20; i++ {
		require.NoError(t, first.Log(context.Background(), AuditRecord{AgentID: "agent-a", Action: "exec:binary:ssh", Type: "executable", Result: "granted"}))
	}
	require.NoError(t, first.Close())

	require.FileExists(t, filepath.Join(dir, auditChainHeadFile))
	require.NoError(t, os.Remove(filepath.Join(dir, auditChainHeadFile)))

	// Non-destructive probe before boot: full replay is clean so it is NOT
	// broken even though the anchor is gone.
	probe := ProbeChainDir(dir)
	require.False(t, probe.Broken, "clean chain without head must not read as broken")
	require.Equal(t, int64(20), probe.LastSeq)

	// Boot reconstructs by full scan and continues the sequence.
	second, err := NewFileChainAuditLogger(dir, FileChainOptions{})
	require.NoError(t, err)
	detected, _, _ := second.TamperInfo()
	require.False(t, detected, "clean chain must not trip the tamper path")
	require.NoError(t, second.Log(context.Background(), AuditRecord{AgentID: "agent-a", Action: "exec:binary:ssh", Type: "executable", Result: "granted"}))
	v, err := second.VerifyChain(context.Background(), AuditChainFilter{})
	require.NoError(t, err)
	require.True(t, v.Verified)
	require.Equal(t, int64(21), v.LastSequence)
	require.NoError(t, second.Close())
}

func TestChainLogger_TamperDetectionRotatesWithGenesis(t *testing.T) {
	dir := t.TempDir()
	first, err := NewFileChainAuditLogger(dir, FileChainOptions{BatchFlush: 5 * time.Millisecond})
	require.NoError(t, err)
	for i := 0; i < 30; i++ {
		require.NoError(t, first.Log(context.Background(), AuditRecord{AgentID: "agent-a", Action: "exec:binary:ssh", Type: "executable", Result: "granted"}))
	}
	require.NoError(t, first.Close())

	// Compromise the tail so the head anchor no longer matches.
	files, err := chainFilesIn(dir)
	require.NoError(t, err)
	last := files[len(files)-1]
	data, err := os.ReadFile(last)
	require.NoError(t, err)
	lastLine := strings.LastIndex(strings.TrimRight(string(data), "\n"), "\n")
	data[lastLine+3] ^= 0xff
	require.NoError(t, os.WriteFile(last, data, 0o640))

	// The probe reports broken.
	require.True(t, ProbeChainDir(dir).Broken)

	second, err := NewFileChainAuditLogger(dir, FileChainOptions{})
	require.NoError(t, err)
	detected, tamperAt, prevHash := second.TamperInfo()
	require.True(t, detected, "broken chain must rotate with a tamper marker")
	require.False(t, tamperAt.IsZero())
	require.Len(t, prevHash, 64)

	// The genesis entry is part of the chain and carries the tamper marker.
	require.NoError(t, second.Log(context.Background(), AuditRecord{AgentID: "agent-a", Action: "exec:binary:ssh", Type: "executable", Result: "granted"}))
	entries, err := second.ReadChain(context.Background(), AuditChainFilter{})
	require.NoError(t, err)

	var genesis *AuditChainEntry
	for i := range entries {
		if entries[i].Record.Action == "chain_integrity" {
			genesis = &entries[i]
			break
		}
	}
	require.NotNil(t, genesis, "tamper genesis entry must exist")
	meta := genesis.Record.Metadata
	require.Equal(t, prevHash, meta["previous_valid_hash"])
	require.NotEmpty(t, meta["tamper_detected_at"])
	require.Equal(t, prevHash, genesis.PreviousHash, "genesis must chain from the last valid hash")

	// The historical tampered record remains as evidence: the full chain still
	// reports broken, but the new rotation file is chained from the last valid
	// prefix (verified below item by item).
	v, err := second.VerifyChain(context.Background(), AuditChainFilter{})
	require.NoError(t, err)
	require.False(t, v.Verified, "tampered historical records remain unreconciled by design")

	// The genesis onward chains correctly: every entry in the rotation file
	// validates against its chained previous hash.
	rotation := filepath.Join(second.dir, second.ws.file)
	rotEntries, partial, err := readChainEntries(rotation)
	require.NoError(t, err)
	require.False(t, partial)
	require.NotEmpty(t, rotEntries)
	require.Equal(t, prevHash, rotEntries[0].PreviousHash)
	prev := prevHash
	for i := range rotEntries {
		require.Equal(t, prev, rotEntries[i].PreviousHash, "rotation link at index %d", i)
		require.Equal(t, chainRecordHash(prev, rotEntries[i].Record), rotEntries[i].RecordHash)
		prev = rotEntries[i].RecordHash
	}
	require.NoError(t, second.Close())
}

func TestChainLogger_StrictEnqueueTimesOutWhenWriterStalled(t *testing.T) {
	// White-box: no writer goroutine means the channel never drains.
	l := &FileChainAuditLogger{
		dir:  t.TempDir(),
		opts: FileChainOptions{QueueSize: 1, Now: time.Now, Timeouts: FileChainTimeouts{Enqueue: 60 * time.Millisecond}},
		ch:   make(chan chainRequest, 1),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	ctx := context.Background()
	require.NoError(t, l.Log(ctx, AuditRecord{AgentID: "a", Action: "exec:binary:ssh", Type: "executable", Result: "granted"}))
	start := time.Now()
	// The second strict record has no queue room and no writer to consume it.
	err := l.Log(ctx, AuditRecord{AgentID: "a", Action: "exec:binary:ssh", Type: "executable", Result: "granted"})
	require.ErrorIs(t, err, ErrAuditUnavailable)
	require.GreaterOrEqual(t, time.Since(start), 45*time.Millisecond)
}

func TestChainLogger_BestEffortDropsAndCounts(t *testing.T) {
	now := time.Now()
	l := newChainLogger(t, &now, FileChainOptions{BestEffort: true})
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		require.NoError(t, l.Log(ctx, AuditRecord{AgentID: "a", Action: "exec:binary:ssh", Type: "executable", Result: "granted"}))
	}
	require.Equal(t, uint64(10), l.DroppedTotal())
	v, err := l.VerifyChain(ctx, AuditChainFilter{})
	require.NoError(t, err)
	require.True(t, v.Verified)
	require.Equal(t, int64(0), v.LastSequence, "best_effort must not persist")
	files, err := chainFilesIn(l.dir)
	require.NoError(t, err)
	require.Empty(t, files)
	require.NoError(t, l.Close())
}

func TestChainLogger_ReadOnlyFileAccessIsBestEffort(t *testing.T) {
	now := time.Now()
	l := newChainLogger(t, &now, FileChainOptions{})
	ctx := context.Background()

	// Read is best-effort: dropped and counted, never persisted.
	require.NoError(t, l.Log(ctx, AuditRecord{Type: "filesystem", Action: "fs:read", Result: "granted", Metadata: map[string]any{"fs_action": "fs:read"}}))
	require.Equal(t, uint64(1), l.DroppedTotal())

	// Mutating file access is strict: persisted.
	require.NoError(t, l.Log(ctx, AuditRecord{Type: "filesystem", Action: "fs:write", Result: "granted", Metadata: map[string]any{"fs_action": "fs:write"}}))
	require.Equal(t, uint64(1), l.DroppedTotal())

	v, err := l.VerifyChain(ctx, AuditChainFilter{})
	require.NoError(t, err)
	require.True(t, v.Verified)
	require.Equal(t, int64(1), v.LastSequence)
	require.NoError(t, l.Close())
}

func TestChainLogger_UnrecognizedTypeFallsBackToStrict(t *testing.T) {
	now := time.Now()
	l := newChainLogger(t, &now, FileChainOptions{})
	ctx := context.Background()
	require.NoError(t, l.Log(ctx, AuditRecord{AgentID: "a", Type: "mystery-family", Action: "some:action", Result: "granted"}))
	v, err := l.VerifyChain(ctx, AuditChainFilter{})
	require.NoError(t, err)
	require.True(t, v.Verified)
	require.Equal(t, int64(1), v.LastSequence)
	require.NoError(t, l.Close())
}

func TestChainLogger_PropertyAgainstModel(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	l, err := NewFileChainAuditLogger(dir, FileChainOptions{BatchFlush: 5 * time.Millisecond, Now: func() time.Time { return now }})
	require.NoError(t, err)
	ctx := context.Background()

	// Model: monotonic sequence + integrity expectation.
	modelCount := 0
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 1500; i++ {
		var ts time.Time
		switch rng.Intn(4) {
		case 0:
			ts = now.AddDate(0, 0, 1) // next day → rotation
		case 1:
			ts = now.AddDate(0, 0, 2)
		default:
			ts = time.Time{} // use the injected clock
		}
		rec := AuditRecord{AgentID: "agent-a", Timestamp: ts, Action: "exec:binary:ssh", Type: "executable", Result: "granted"}
		if rng.Intn(10) < 9 {
			require.NoError(t, l.Log(ctx, rec))
			modelCount++
		}
		// Occasionally sweep the queue via a flush barrier.
		if i%100 == 0 {
			require.NoError(t, l.flushToDisk(ctx))
		}
	}
	v, err := l.VerifyChain(ctx, AuditChainFilter{})
	require.NoError(t, err)
	require.True(t, v.Verified, "model chain must verify: %s", v.Failure)
	require.Equal(t, int64(modelCount), v.LastSequence)
	require.NoError(t, l.Close())

	// Re-opening boots to the same sequence.
	again, err := NewFileChainAuditLogger(dir, FileChainOptions{})
	require.NoError(t, err)
	detected, _, _ := again.TamperInfo()
	require.False(t, detected)
	require.Equal(t, int64(modelCount), again.ws.seq)
	require.NoError(t, again.Close())
}

func TestChainLogger_ConcurrentProducersStrictMonotonic(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	l, err := NewFileChainAuditLogger(dir, FileChainOptions{QueueSize: 4096, BatchFlush: 5 * time.Millisecond, Now: func() time.Time { return now }})
	require.NoError(t, err)
	ctx := context.Background()

	const goroutines = 32
	const perGoroutine = 500
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				if err := l.Log(ctx, AuditRecord{AgentID: fmt.Sprintf("agent-%d", g), Action: "exec:binary:ssh", Type: "executable", Result: "granted"}); err != nil {
					t.Errorf("concurrent Log: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	v, err := l.VerifyChain(ctx, AuditChainFilter{})
	require.NoError(t, err)
	require.True(t, v.Verified, "concurrent chain must verify: %s", v.Failure)
	require.Equal(t, int64(goroutines*perGoroutine), v.LastSequence)
	require.Equal(t, goroutines*perGoroutine, v.EntryCount)

	// Monotonicity is proven by VerifyChain; assert it explicitly as well.
	entries, err := l.ReadChain(ctx, AuditChainFilter{})
	require.NoError(t, err)
	prevSeq := int64(0)
	for i := range entries {
		if entries[i].Sequence <= prevSeq {
			t.Fatalf("sequence not strictly monotonic: %d after %d", entries[i].Sequence, prevSeq)
		}
		prevSeq = entries[i].Sequence
	}
	require.NoError(t, l.Close())
}

func TestChainLogger_QueryFilteringAndReadChainLimit(t *testing.T) {
	now := time.Now()
	l := newChainLogger(t, &now, FileChainOptions{})
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		require.NoError(t, l.Log(ctx, AuditRecord{AgentID: "agent-a", Action: "exec:binary:ssh", Type: "executable", Result: "granted"}))
		require.NoError(t, l.Log(ctx, AuditRecord{AgentID: "agent-b", Action: "net:egress:tcp", Type: "network", Result: "denied", Metadata: map[string]any{"reason": "private"}}))
	}

	networkOnly, err := l.Query(ctx, AuditQuery{Type: "network"})
	require.NoError(t, err)
	require.Len(t, networkOnly, 10)
	for _, r := range networkOnly {
		require.Equal(t, "network", r.Type)
	}

	limited, err := l.ReadChain(ctx, AuditChainFilter{Limit: 5})
	require.NoError(t, err)
	require.Len(t, limited, 5)
	require.NoError(t, l.Close())
}

func TestChainLogger_CloseIsIdempotent(t *testing.T) {
	now := time.Now()
	l := newChainLogger(t, &now, FileChainOptions{})
	require.NoError(t, l.Close())
	require.NoError(t, l.Close())
	// Log after close is refused.
	err := l.Log(context.Background(), AuditRecord{AgentID: "a", Action: "exec:binary:ssh", Type: "executable", Result: "granted"})
	require.Error(t, err)
}

func TestChainLogger_ProbeFreshDir(t *testing.T) {
	probe := ProbeChainDir(filepath.Join(t.TempDir(), "audit", "agent-x"))
	require.False(t, probe.Present)
	require.False(t, probe.Broken)
}

// BenchmarkChainAppend measures the producer-side cost of a strict Log call
// with the writer running (NFR-4: producer-side p99 ≤ 2 ms). The benchmark
// asserts the write path stays above the 10 000 records/min floor.
func BenchmarkChainAppend(b *testing.B) {
	now := time.Now()
	l, err := NewFileChainAuditLogger(b.TempDir(), FileChainOptions{Now: func() time.Time { return now }})
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	ctx := context.Background()
	rec := AuditRecord{AgentID: "agent-bench", Action: "exec:binary:ssh", Type: "executable", Result: "granted"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := l.Log(ctx, rec); err != nil {
			b.Fatalf("Log: %v", err)
		}
	}
	b.StopTimer()
}

func TestChainLogger_ThroughputFloor(t *testing.T) {
	if testing.Short() {
		t.Skip("throughput floor exercised in the race suite")
	}
	now := time.Now()
	l := newChainLogger(t, &now, FileChainOptions{})
	ctx := context.Background()
	const target = 600 // well under the 10_000/min floor in absolute terms
	start := time.Now()
	for i := 0; i < target; i++ {
		require.NoError(t, l.Log(ctx, AuditRecord{AgentID: "a", Action: "exec:binary:ssh", Type: "executable", Result: "granted"}))
	}
	elapsed := time.Since(start)
	require.NoError(t, l.Close())
	rate := float64(target) / elapsed.Minutes()
	require.GreaterOrEqual(t, rate, 10_000.0, "append throughput too low: %.0f rec/min", rate)
	t.Logf("chain append rate: %.0f records/min", rate)
}

func TestChainLogger_JSONLineShape(t *testing.T) {
	// The on-disk line shape matches the §5.4 data model: sequence, record,
	// previous_hash, record_hash for the genesis entry.
	now := time.Now()
	l := newChainLogger(t, &now, FileChainOptions{})
	ctx := context.Background()
	require.NoError(t, l.Log(ctx, goldenRecord()))
	require.NoError(t, l.flushToDisk(ctx))

	files, err := chainFilesIn(l.dir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	data, err := os.ReadFile(files[0])
	require.NoError(t, err)

	var entry AuditChainEntry
	require.NoError(t, json.Unmarshal(data, &entry))
	require.Equal(t, int64(1), entry.Sequence)
	require.Equal(t, auditChainGenesisHash, entry.PreviousHash)
	require.Len(t, entry.RecordHash, 64)
	require.Equal(t, goldenRecord().Action, entry.Record.Action)
	require.NoError(t, l.Close())
}
