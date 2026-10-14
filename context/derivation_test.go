package context

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOriginDerivationCreatesSingleOriginStep(t *testing.T) {
	chain := OriginDerivation("test-system")

	require.False(t, chain.IsEmpty())
	require.Equal(t, 1, chain.Depth())

	origin := chain.Origin()
	require.NotNil(t, origin)
	require.Equal(t, "origin", origin.Transform)
	require.Equal(t, "test-system", origin.SourceSystem)
	require.Zero(t, origin.LossMagnitude)
	require.Empty(t, origin.ParentID)
	require.Empty(t, origin.Detail)

	// Content-addressed IDs are 16 hex characters.
	require.Len(t, origin.ID, 16)
	_, err := hex.DecodeString(string(origin.ID))
	require.NoError(t, err, "origin ID must be hex-encoded")
}

func TestGenerateDerivationIDIsDeterministicPerInputs(t *testing.T) {
	ts := time.Now().UTC()

	id1 := generateDerivationID("parent-1", "compress", ts)
	id2 := generateDerivationID("parent-1", "compress", ts)
	require.Equal(t, id1, id2, "same parent/transform/timestamp must produce the same ID")

	require.NotEqual(t, id1, generateDerivationID("parent-2", "compress", ts), "different parent must change the ID")
	require.NotEqual(t, id1, generateDerivationID("parent-1", "truncate", ts), "different transform must change the ID")
	require.NotEqual(t, id1, generateDerivationID("parent-1", "compress", ts.Add(time.Second)), "different timestamp must change the ID")

	// The ID is the 16-hex-char prefix of the documented sha256 input.
	sum := sha256.Sum256([]byte("parent-1:compress:" + fmt.Sprintf("%d", ts.UnixNano())))
	require.Equal(t, DerivationID(hex.EncodeToString(sum[:])[:16]), id1)
}

func TestDeriveAppendsStepLinkedToParent(t *testing.T) {
	chain := OriginDerivation("sys")
	first := chain.Steps[0]

	derived := chain.Derive("summarize", "llm", 0.25, "condensed")
	require.Len(t, derived.Steps, 2)
	require.Equal(t, 2, derived.Depth())

	step := derived.Steps[1]
	require.Equal(t, first.ID, step.ParentID, "derived step must link to the previous step ID")
	require.Equal(t, "summarize", step.Transform)
	require.Equal(t, "llm", step.SourceSystem)
	require.InDelta(t, 0.25, step.LossMagnitude, 0)
	require.Equal(t, "condensed", step.Detail)
	require.Len(t, step.ID, 16)

	// The original chain must be untouched (value receiver returns a new chain).
	require.Len(t, chain.Steps, 1)
}

func TestDeriveClampsLossMagnitude(t *testing.T) {
	chain := DerivationChain{}

	below := chain.Derive("t1", "sys", -0.5, "")
	require.InDelta(t, 0.0, below.Steps[0].LossMagnitude, 0, "negative loss clamps to 0")

	above := chain.Derive("t2", "sys", 1.75, "")
	require.InDelta(t, 1.0, above.Steps[0].LossMagnitude, 0, "loss above 1 clamps to 1")
}

func TestTotalLossComposesMultiplicatively(t *testing.T) {
	require.InDelta(t, 0.0, DerivationChain{}.TotalLoss(), 0, "empty chain has zero loss")

	// 1 - (1-0.1)(1-0.2) = 0.28
	chain := DerivationChain{Steps: []DerivationStep{
		{ID: "a", LossMagnitude: 0.1},
		{ID: "b", LossMagnitude: 0.2},
	}}
	require.InDelta(t, 0.28, chain.TotalLoss(), 1e-9)

	// A fully-lossy step pins the total at 1.
	full := DerivationChain{Steps: []DerivationStep{
		{ID: "a", LossMagnitude: 0.5},
		{ID: "b", LossMagnitude: 1.0},
	}}
	require.InDelta(t, 1.0, full.TotalLoss(), 0)

	// Hand-built chains with out-of-range magnitudes are clamped, never negative.
	negative := DerivationChain{Steps: []DerivationStep{{ID: "a", LossMagnitude: -0.5}}}
	require.InDelta(t, 0.0, negative.TotalLoss(), 0)

	// Magnitudes above 1 can push the raw product negative; the total caps at 1.
	over := DerivationChain{Steps: []DerivationStep{{ID: "a", LossMagnitude: 2.0}}}
	require.InDelta(t, 1.0, over.TotalLoss(), 0)
}

func TestEmptyChainAccessors(t *testing.T) {
	empty := DerivationChain{}

	require.True(t, empty.IsEmpty())
	require.Nil(t, empty.Origin())
	require.Equal(t, 0, empty.Depth())
	require.InDelta(t, 0.0, empty.TotalLoss(), 0)
	require.Empty(t, empty.LastTransform())
	require.True(t, empty.LastTimestamp().IsZero())
	require.Equal(t, DerivationSummary{
		Depth:        0,
		TotalLoss:    0,
		OriginSystem: "",
	}, empty.Summary())
}

func TestChainAccessorsReturnLastStepFields(t *testing.T) {
	ts1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 2, 2, 3, 4, 5, 0, time.UTC)
	chain := DerivationChain{Steps: []DerivationStep{
		{ID: "a", Transform: "ingest", SourceSystem: "s1", Timestamp: ts1},
		{ID: "b", Transform: "redact", SourceSystem: "s2", Timestamp: ts2},
	}}

	origin := chain.Origin()
	require.NotNil(t, origin)
	require.Equal(t, "ingest", origin.Transform)
	require.Equal(t, "s1", origin.SourceSystem)

	require.Equal(t, "redact", chain.LastTransform())
	require.Equal(t, ts2, chain.LastTimestamp())
	require.False(t, chain.IsEmpty())

	summary := chain.Summary()
	require.Equal(t, 2, summary.Depth)
	require.Equal(t, "s1", summary.OriginSystem)
	require.Equal(t, "redact", summary.LastTransform)
	require.InDelta(t, 0.0, summary.TotalLoss, 0)
}

func TestCloneProducesIndependentCopy(t *testing.T) {
	require.Equal(t, DerivationChain{Steps: []DerivationStep{}}, DerivationChain{}.Clone(),
		"cloning an empty chain yields an empty, non-nil step slice")

	original := OriginDerivation("sys")
	clone := original.Clone()
	require.Equal(t, original, clone)

	clone.Steps[0].Transform = "mutated"
	clone.Steps = append(clone.Steps, DerivationStep{ID: "x"})
	require.Equal(t, "origin", original.Steps[0].Transform)
	require.Len(t, original.Steps, 1, "appending to the clone must not affect the original")
}
