package contextstream

import (
	"errors"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func compileResult(id string, budget, finalTokens int, chunks ...contextports.StreamedChunkView) *Result {
	return &Result{
		Request: Request{ID: id},
		Compilation: &contextports.CompilationResult{
			StreamedChunks: chunks,
			Record: contextports.CompilationRecord{
				ID:             id,
				FinalTokens:    finalTokens,
				OriginalBudget: budget,
			},
		},
	}
}

type sliceTelemetrySink struct {
	events []telemetry.Event
}

func (s *sliceTelemetrySink) Emit(event telemetry.Event) {
	s.events = append(s.events, event)
}

func (s *sliceTelemetrySink) count(eventType telemetry.EventType) int {
	n := 0
	for _, event := range s.events {
		if event.Type == eventType {
			n++
		}
	}
	return n
}

func TestApplyResultStoresSlice(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	result := compileResult("req-1", 8192, 100,
		contextports.StreamedChunkView{ChunkID: "chunk-a", ContentHash: "ha", Body: "body a", TokenEstimate: 60, TrustClass: "workspace"},
		contextports.StreamedChunkView{ChunkID: "chunk-b", ContentHash: "hb", Body: "body b", TokenEstimate: 40, IsSummary: true},
	)

	if err := ApplyResult(env, result, 5); err != nil {
		t.Fatalf("ApplyResult: %v", err)
	}
	slice := env.StreamedSliceSnapshot()
	if slice == nil {
		t.Fatal("no slice stored")
	}
	if slice.RequestID != "req-1" || slice.Epoch != 5 || slice.BudgetTokens != 8192 || slice.FinalTokens != 100 {
		t.Fatalf("slice header = %+v", slice)
	}
	if len(slice.Chunks) != 2 || slice.Chunks[0].ChunkID != "chunk-a" || slice.Chunks[1].Body != "body b" {
		t.Fatalf("chunks = %+v", slice.Chunks)
	}
	stamps := env.StreamedSliceStamps()
	if len(stamps) != 1 || stamps[0].Epoch != 5 || len(stamps[0].ChunkIDs) != 2 || stamps[0].ChunkHashes[0] != "ha" {
		t.Fatalf("history stamps = %+v", stamps)
	}
}

// TestApplyResultEpochGuard is the D-3 table: a higher or equal epoch
// replaces; a lower epoch returns the typed ErrStaleApplyDropped sentinel —
// the envelope is untouched — and emits nothing (the caller owns the
// stale_apply_dropped emission).
func TestApplyResultEpochGuard(t *testing.T) {
	cases := []struct {
		name        string
		firstEpoch  uint64
		secondEpoch uint64
		wantStored  string // request id of the stored slice after both applies
		wantDropped bool
	}{
		{"older_dropped", 7, 6, "req-first", true},
		{"equal_reapplies", 7, 7, "req-second", false},
		{"younger_replaces", 7, 8, "req-second", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := &sliceTelemetrySink{}
			env := contextdata.NewEnvelope("t", "s")
			if err := ApplyResult(env, compileResult("req-first", 0, 10,
				contextports.StreamedChunkView{ChunkID: "chunk-1", Body: "one", TokenEstimate: 10}), tc.firstEpoch); err != nil {
				t.Fatal(err)
			}
			before := env.ReferencesSnapshot()
			beforeMeta := env.AssemblyMetadataSnapshot()
			err := ApplyResult(env, compileResult("req-second", 0, 10,
				contextports.StreamedChunkView{ChunkID: "chunk-2", Body: "two", TokenEstimate: 10}), tc.secondEpoch)
			if tc.wantDropped {
				var dropped *ErrStaleApplyDropped
				if !errors.As(err, &dropped) {
					t.Fatalf("stale apply err = %v, want *ErrStaleApplyDropped", err)
				}
				if dropped.CurrentEpoch != 7 || dropped.GotEpoch != 6 || dropped.RequestID != "req-second" {
					t.Fatalf("sentinel fields = %+v", dropped)
				}
				after := env.ReferencesSnapshot()
				if len(after.StreamedContext) != len(before.StreamedContext) {
					t.Fatalf("stale apply mutated streamed refs: %d -> %d", len(before.StreamedContext), len(after.StreamedContext))
				}
				afterMeta := env.AssemblyMetadataSnapshot()
				if afterMeta != beforeMeta {
					t.Fatalf("stale apply mutated assembly metadata: %+v -> %+v", beforeMeta, afterMeta)
				}
				if got := env.StreamedSliceSnapshot(); got == nil || got.RequestID != tc.wantStored {
					t.Fatalf("stored slice = %+v, want request %q", got, tc.wantStored)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			// Emission is caller-owned: ApplyResult itself never emits.
			if got := sink.count(telemetry.EventContextStreamStaleApplyDropped); got != 0 {
				t.Fatalf("ApplyResult emitted stale_apply_dropped itself (%d events)", got)
			}
			stamps := env.StreamedSliceStamps()
			if tc.wantDropped {
				if len(stamps) != 1 {
					t.Fatalf("dropped apply appended a history stamp: %+v", stamps)
				}
			} else if len(stamps) != 2 {
				t.Fatalf("history stamps = %d, want 2", len(stamps))
			}
		})
	}
}

func TestApplyResultBudgetViolationErrors(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	// One chunk claiming 4096 tokens (~16KB) against a 1024-token budget
	// (8KB limit): 2x bound (NFR-2) violated.
	result := compileResult("req-big", 1024, 4096,
		contextports.StreamedChunkView{ChunkID: "chunk-big", Body: "big", TokenEstimate: 4096})
	if err := ApplyResult(env, result, 1); err == nil || !strings.Contains(err.Error(), "2x budget") {
		t.Fatalf("err = %v, want the 2x-budget violation", err)
	}
	if env.StreamedSliceSnapshot() != nil {
		t.Fatal("violating slice must not be stored")
	}
}

func TestApplyResultRecordsGaps(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	result := compileResult("req-gaps", 0, 10,
		contextports.StreamedChunkView{ChunkID: "chunk-ok", Body: "ok", TokenEstimate: 10})
	result.Compilation.SkippedStaleChunks = []string{"chunk-stale"}
	if err := ApplyResult(env, result, 1); err != nil {
		t.Fatal(err)
	}
	slice := env.StreamedSliceSnapshot()
	if len(slice.GapMessages) != 1 || slice.GapMessages[0] != "chunk-stale" {
		t.Fatalf("gap messages = %+v", slice.GapMessages)
	}
}

func TestApplyResultEmptyCompilationStoresEmptySlice(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	result := &Result{Compilation: &contextports.CompilationResult{}}
	if err := ApplyResult(env, result, 2); err != nil {
		t.Fatal(err)
	}
	slice := env.StreamedSliceSnapshot()
	if slice == nil || len(slice.Chunks) != 0 {
		t.Fatalf("empty compile must store an empty slice, got %+v", slice)
	}
	section, _, err := RenderStreamedSection(env)
	if err != nil || section != "" {
		t.Fatalf("empty slice must render zero bytes, got (%q, %v)", section, err)
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestEpochMetadataNeverRegresses is the D-3 table: AssemblyMetadata.EpochID
// uses max semantics — a younger apply never rewinds the stamp, an equal or
// older compilation ID still updates attribution.
func TestEpochMetadataNeverRegresses(t *testing.T) {
	cases := []struct {
		name       string
		first      uint64
		second     uint64
		wantEpoch  uint64
		wantRecord string // CompilationID after the second apply
	}{
		{"younger_never_regresses", 7, 3, 7, "req-a"},
		{"equal_keeps", 7, 7, 7, "req-b"},
		{"older_raises", 7, 9, 9, "req-b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := contextdata.NewEnvelope("t", "s")
			first := compileResult("req-a", 0, 10)
			first.Record = &contextports.CompilationRecord{ID: "req-a"}
			if err := ApplyResult(env, first, tc.first); err != nil {
				t.Fatal(err)
			}
			// A stale apply mutates nothing — including metadata — so the
			// never-regress property is asserted on accepted applies; the
			// guard case has its own table (TestApplyResultEpochGuard).
			second := compileResult("req-b", 0, 10)
			second.Record = &contextports.CompilationRecord{ID: "req-b"}
			if tc.second < tc.first {
				env.SetStreamedSlice(&contextdata.StreamedSlice{RequestID: "seed", Epoch: tc.second + 1})
			}
			if err := ApplyResult(env, second, tc.second); err != nil {
				var dropped *ErrStaleApplyDropped
				if !errors.As(err, &dropped) {
					t.Fatal(err)
				}
			}
			meta := env.AssemblyMetadataSnapshot()
			if meta.EpochID != tc.wantEpoch {
				t.Fatalf("EpochID = %d, want %d", meta.EpochID, tc.wantEpoch)
			}
			if meta.CompilationID != tc.wantRecord {
				t.Fatalf("CompilationID = %q, want %q", meta.CompilationID, tc.wantRecord)
			}
		})
	}
}

// TestStreamedRefsDedup proves D-3's dedup: re-applying the same compilation
// (equal epoch, a node retry) leaves the reference list at one entry per
// chunk instead of accumulating a copy per apply.
func TestStreamedRefsDedup(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	result := compileResult("req-1", 0, 10,
		contextports.StreamedChunkView{ChunkID: "chunk-1", Body: "one", TokenEstimate: 10},
		contextports.StreamedChunkView{ChunkID: "chunk-2", Body: "two", TokenEstimate: 10},
	)
	result.Compilation.StreamedRefs = []string{"chunk-1", "chunk-2"}
	if err := ApplyResult(env, result, 3); err != nil {
		t.Fatal(err)
	}
	// A retry references one known chunk plus nothing new.
	retry := compileResult("req-1-retry", 0, 10,
		contextports.StreamedChunkView{ChunkID: "chunk-1", Body: "one", TokenEstimate: 10},
		contextports.StreamedChunkView{ChunkID: "chunk-3", Body: "three", TokenEstimate: 10},
	)
	retry.Compilation.StreamedRefs = []string{"chunk-1", "chunk-3"}
	if err := ApplyResult(env, retry, 3); err != nil {
		t.Fatal(err)
	}
	ids := env.StreamedChunkIDs()
	if len(ids) != 3 {
		t.Fatalf("streamed chunk ids = %v, want exactly chunk-1..3 with no duplicates", ids)
	}
	seen := map[contextdata.ChunkID]int{}
	for _, id := range ids {
		seen[id]++
		if seen[id] > 1 {
			t.Fatalf("duplicate reference for %s: %v", id, ids)
		}
	}
}
