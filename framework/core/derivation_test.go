package core

import (
	"math"
	"testing"
	"time"
)

func TestOriginDerivation(t *testing.T) {
	chain := OriginDerivation("retrieval")

	if chain.IsEmpty() {
		t.Error("origin chain should not be empty")
	}

	if chain.Depth() != 1 {
		t.Errorf("expected depth 1, got %d", chain.Depth())
	}

	origin := chain.Origin()
	if origin == nil {
		t.Fatal("origin should not be nil")
	}

	if origin.SourceSystem != "retrieval" {
		t.Errorf("expected source_system 'retrieval', got '%s'", origin.SourceSystem)
	}

	if origin.Transform != "origin" {
		t.Errorf("expected transform 'origin', got '%s'", origin.Transform)
	}

	if origin.LossMagnitude != 0.0 {
		t.Errorf("expected loss magnitude 0.0, got %f", origin.LossMagnitude)
	}
}

func TestDeriveChain(t *testing.T) {
	chain := OriginDerivation("retrieval")

	// Add first transformation
	chain = chain.Derive("chunk", "retrieval", 0.1, "split by heading")

	if chain.Depth() != 2 {
		t.Errorf("expected depth 2 after first derive, got %d", chain.Depth())
	}

	// Add second transformation
	chain = chain.Derive("compress_truncate", "contextmgr", 0.3, "")

	if chain.Depth() != 3 {
		t.Errorf("expected depth 3 after second derive, got %d", chain.Depth())
	}

	lastStep := chain.Steps[len(chain.Steps)-1]
	if lastStep.Transform != "compress_truncate" {
		t.Errorf("expected last transform 'compress_truncate', got '%s'", lastStep.Transform)
	}

	if lastStep.SourceSystem != "contextmgr" {
		t.Errorf("expected source_system 'contextmgr', got '%s'", lastStep.SourceSystem)
	}
}

func TestDerivationParentLinks(t *testing.T) {
	chain := OriginDerivation("retrieval")
	chain = chain.Derive("chunk", "retrieval", 0.1, "")
	chain = chain.Derive("pack", "retrieval", 0.05, "")

	// Verify parent links
	if chain.Steps[1].ParentID == "" {
		t.Error("step 1 should have a parent ID")
	}

	if chain.Steps[2].ParentID == "" {
		t.Error("step 2 should have a parent ID")
	}

	if chain.Steps[1].ParentID != chain.Steps[0].ID {
		t.Error("step 1's parent should be step 0's ID")
	}

	if chain.Steps[2].ParentID != chain.Steps[1].ID {
		t.Error("step 2's parent should be step 1's ID")
	}
}

func TestTotalLoss(t *testing.T) {
	tests := []struct {
		name      string
		losses    []float64
		expected  float64
		tolerance float64
	}{
		{"no loss", []float64{0.0}, 0.0, 0.0001},
		{"single step", []float64{0.5}, 0.5, 0.0001},
		{"two steps 0.5 each", []float64{0.5, 0.5}, 0.75, 0.0001},
		{"two steps 0.3, 0.2", []float64{0.3, 0.2}, 0.44, 0.0001},
		{"near 1.0", []float64{0.9, 0.9}, 0.99, 0.0001},
		{"very near 1.0", []float64{0.95, 0.95}, 0.9975, 0.0001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := OriginDerivation("test")
			for _, loss := range tt.losses {
				chain = chain.Derive("test", "test", loss, "")
			}

			got := chain.TotalLoss()
			if math.Abs(got-tt.expected) > tt.tolerance {
				t.Errorf("TotalLoss() = %f, want %f", got, tt.expected)
			}
		})
	}
}

func TestLossMagnitudeNormalization(t *testing.T) {
	chain := OriginDerivation("test")

	// Values below 0 should be clamped to 0
	chain = chain.Derive("test", "test", -0.5, "")
	if chain.Steps[1].LossMagnitude != 0.0 {
		t.Errorf("negative loss should be clamped to 0, got %f", chain.Steps[1].LossMagnitude)
	}

	// Values above 1 should be clamped to 1
	chain = chain.Derive("test", "test", 1.5, "")
	if chain.Steps[2].LossMagnitude != 1.0 {
		t.Errorf("loss > 1 should be clamped to 1, got %f", chain.Steps[2].LossMagnitude)
	}
}

func TestDerivationClone(t *testing.T) {
	original := OriginDerivation("test")
	original = original.Derive("step1", "test", 0.1, "")
	original = original.Derive("step2", "test", 0.2, "")

	cloned := original.Clone()

	if cloned.Depth() != original.Depth() {
		t.Errorf("cloned depth %d != original depth %d", cloned.Depth(), original.Depth())
	}

	// Verify they are deep copies (modifying clone doesn't affect original)
	cloned.Steps[0].Detail = "modified"
	if original.Steps[0].Detail == "modified" {
		t.Error("clone modification affected original")
	}
}

func TestDerivationIsEmpty(t *testing.T) {
	empty := DerivationChain{Steps: []DerivationStep{}}
	if !empty.IsEmpty() {
		t.Error("chain with no steps should be empty")
	}

	chain := OriginDerivation("test")
	if chain.IsEmpty() {
		t.Error("chain with origin should not be empty")
	}
}

func TestLastTransform(t *testing.T) {
	chain := DerivationChain{Steps: []DerivationStep{}}
	if chain.LastTransform() != "" {
		t.Error("empty chain should have empty last transform")
	}

	chain = OriginDerivation("test")
	if chain.LastTransform() != "origin" {
		t.Errorf("expected 'origin', got '%s'", chain.LastTransform())
	}

	chain = chain.Derive("chunk", "retrieval", 0.1, "")
	if chain.LastTransform() != "chunk" {
		t.Errorf("expected 'chunk', got '%s'", chain.LastTransform())
	}
}

func TestLastTimestamp(t *testing.T) {
	empty := DerivationChain{Steps: []DerivationStep{}}
	if !empty.LastTimestamp().IsZero() {
		t.Error("empty chain should return zero time")
	}

	chain := OriginDerivation("test")
	ts := chain.LastTimestamp()
	if ts.IsZero() {
		t.Error("origin chain should have non-zero timestamp")
	}

	// Timestamp should be close to now
	now := time.Now().UTC()
	if ts.After(now.Add(time.Second)) || ts.Before(now.Add(-time.Second)) {
		t.Errorf("timestamp too far from now: %v vs %v", ts, now)
	}
}

func TestDerivationSummary(t *testing.T) {
	chain := OriginDerivation("retrieval")
	chain = chain.Derive("chunk", "retrieval", 0.1, "")
	chain = chain.Derive("compress_truncate", "contextmgr", 0.3, "")

	summary := chain.Summary()

	if summary.Depth != 3 {
		t.Errorf("expected depth 3, got %d", summary.Depth)
	}

	if summary.OriginSystem != "retrieval" {
		t.Errorf("expected origin_system 'retrieval', got '%s'", summary.OriginSystem)
	}

	if summary.LastTransform != "compress_truncate" {
		t.Errorf("expected last_transform 'compress_truncate', got '%s'", summary.LastTransform)
	}

	// Total loss should be approximately 1 - (0.9 * 0.7) = 0.37
	expectedLoss := 1.0 - (0.9 * 0.7)
	if math.Abs(summary.TotalLoss-expectedLoss) > 0.01 {
		t.Errorf("expected loss ~%.2f, got %.2f", expectedLoss, summary.TotalLoss)
	}
}

func TestMemoryContextItemDerivation(t *testing.T) {
	// Create content longer than 250 chars to trigger compression
	longContent := "This is a very long piece of content that will definitely be compressed because it is much longer than the 250 character limit. We need at least 300 characters to ensure meaningful compression happens. Here is some more text to make the content long enough. The compression will reduce this significantly."

	item := &MemoryContextItem{
		Source:  "memory",
		Content: longContent,
	}

	// Before compression, should have no derivation
	if item.Derivation() != nil {
		t.Error("new item should have no derivation")
	}

	// Compress the item
	compressed, err := item.Compress()
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}

	mci := compressed.(*MemoryContextItem)
	deriv := mci.Derivation()
	if deriv == nil {
		t.Error("compressed item should have derivation")
	}

	if deriv.Depth() != 1 {
		t.Errorf("expected depth 1 after compress, got %d", deriv.Depth())
	}

	if deriv.Steps[0].Transform != "compress_truncate" {
		t.Errorf("expected transform 'compress_truncate', got '%s'", deriv.Steps[0].Transform)
	}
}

func TestRetrievalContextItemDerivation(t *testing.T) {
	// Create content longer than 250 chars to trigger compression
	longContent := "This is a very long piece of retrieval evidence content that will definitely be compressed because it is much longer than the 250 character limit. We need at least 300 characters to ensure meaningful compression happens. Here is some more retrieval text to make the content long enough. The compression will reduce this significantly."

	item := &RetrievalContextItem{
		Source:  "retrieval",
		Content: longContent,
	}

	// Add an origin derivation first using WithDerivation
	origin := OriginDerivation("retrieval")
	item = item.WithDerivation(origin).(*RetrievalContextItem)

	// Compress the item
	compressed, err := item.Compress()
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}

	rci := compressed.(*RetrievalContextItem)
	deriv := rci.Derivation()
	if deriv == nil {
		t.Error("compressed item should have derivation")
	}

	// Should have origin + compress step
	if deriv.Depth() < 2 {
		t.Errorf("expected depth >= 2 after compress, got %d", deriv.Depth())
	}

	lastTransform := deriv.LastTransform()
	if lastTransform != "compress_truncate" {
		t.Errorf("expected last transform 'compress_truncate', got '%s'", lastTransform)
	}
}

func TestCapabilityResultContextItemDerivation(t *testing.T) {
	// Create a long output to ensure compression happens
	longOutput := "This is a very long output from the tool that will definitely be compressed because it is much longer than the 250 character limit. We need at least 300 characters to ensure meaningful compression happens. Here is some more output text to make the content long enough. The compression will reduce this significantly."

	item := &CapabilityResultContextItem{
		ToolName: "test_tool",
		Result: &ToolResult{
			Success: true,
			Data: map[string]interface{}{
				"output": longOutput,
			},
		},
	}

	// Compress the item
	compressed, err := item.Compress()
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}

	crci := compressed.(*CapabilityResultContextItem)
	deriv := crci.Derivation()
	if deriv == nil {
		t.Error("compressed item should have derivation")
	}

	if deriv.Depth() != 1 {
		t.Errorf("expected depth 1 after compress, got %d", deriv.Depth())
	}

	if deriv.Steps[0].Transform != "compress_summarize" {
		t.Errorf("expected transform 'compress_summarize', got '%s'", deriv.Steps[0].Transform)
	}
}

func TestWithDerivation(t *testing.T) {
	item := &MemoryContextItem{
		Source:  "memory",
		Content: "test",
	}

	chain := OriginDerivation("test")
	updated := item.WithDerivation(chain)

	mci := updated.(*MemoryContextItem)
	deriv := mci.Derivation()
	if deriv == nil {
		t.Error("item should have derivation after WithDerivation")
	}

	if deriv.Depth() != 1 {
		t.Errorf("expected depth 1, got %d", deriv.Depth())
	}

	// Original should be unchanged
	if item.Derivation() != nil {
		t.Error("original item should not be modified")
	}
}

func TestMemoryRecordEnvelopeDerivation(t *testing.T) {
	envelope := &MemoryRecordEnvelope{
		Key:  "test",
		Text: "content",
	}

	if envelope.Derivation != nil {
		t.Error("new envelope should have nil derivation")
	}

	// Add derivation
	chain := OriginDerivation("memory")
	chain = chain.Derive("memory_recall", "memory", 0.0, "")
	envelope.Derivation = &chain

	if envelope.Derivation == nil {
		t.Error("envelope should have derivation")
	}

	if envelope.Derivation.Depth() != 2 {
		t.Errorf("expected depth 2, got %d", envelope.Derivation.Depth())
	}
}

func TestDerivationIDGeneration(t *testing.T) {
	id1 := generateDerivationID("parent1", "transform1", time.Now().UTC())
	id2 := generateDerivationID("parent1", "transform1", time.Now().UTC())

	// Different timestamps should generate different IDs
	time.Sleep(time.Nanosecond)
	id3 := generateDerivationID("parent1", "transform1", time.Now().UTC())

	if id1 == id2 {
		t.Error("IDs with different timestamps should differ")
	}

	if id1 == id3 {
		t.Error("IDs with different timestamps should differ")
	}

	// Same inputs should generate same ID
	now := time.Now().UTC()
	id4 := generateDerivationID("parent1", "transform1", now)
	id5 := generateDerivationID("parent1", "transform1", now)

	if id4 != id5 {
		t.Error("IDs with same inputs should be identical")
	}
}
