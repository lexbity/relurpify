package contextstream

import (
	"context"
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

	if err := ApplyResult(context.Background(), env, result, 5); err != nil {
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

// TestApplyResultEpochGuard is the D-4 table: a higher or equal epoch
// replaces; a lower epoch is dropped, counted, and never rendered.
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
			ctx := telemetry.WithTelemetry(context.Background(), sink)
			env := contextdata.NewEnvelope("t", "s")
			if err := ApplyResult(ctx, env, compileResult("req-first", 0, 10,
				contextports.StreamedChunkView{ChunkID: "chunk-1", Body: "one", TokenEstimate: 10}), tc.firstEpoch); err != nil {
				t.Fatal(err)
			}
			if err := ApplyResult(ctx, env, compileResult("req-second", 0, 10,
				contextports.StreamedChunkView{ChunkID: "chunk-2", Body: "two", TokenEstimate: 10}), tc.secondEpoch); err != nil {
				t.Fatal(err)
			}
			slice := env.StreamedSliceSnapshot()
			if slice == nil || slice.RequestID != tc.wantStored {
				t.Fatalf("stored slice = %+v, want request %q", slice, tc.wantStored)
			}
			if got := sink.count(telemetry.EventContextStreamStaleApplyDropped); got != boolToInt(tc.wantDropped) {
				t.Fatalf("stale_apply_dropped events = %d, want %d", got, boolToInt(tc.wantDropped))
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
	if err := ApplyResult(context.Background(), env, result, 1); err == nil || !strings.Contains(err.Error(), "2x budget") {
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
	if err := ApplyResult(context.Background(), env, result, 1); err != nil {
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
	if err := ApplyResult(context.Background(), env, result, 2); err != nil {
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
