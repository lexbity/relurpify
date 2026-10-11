package agentgraph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// TestCoordinatorSeedsEnvelopeCompileIdentity proves the coordinator seeds the
// envelope with the run's compile identity (RunID + opening EpochID) at
// construction: every request builder reads one consistent source from the
// FIRST compile, before any barrier has closed, and the metadata mirror can
// never disagree with the request stamp (the epoch-0 cache-key defect).
func TestCoordinatorSeedsEnvelopeCompileIdentity(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "sess-1")
	ctx := contextdata.WithEnvelope(context.Background(), env)

	coord := NewEpochCoordinator(ctx, nil, nil)
	require.NotEmpty(t, coord.RunID(), "a constructed coordinator always carries a run identity")

	meta := env.AssemblyMetadataSnapshot()
	require.Equal(t, uint64(1), meta.EpochID, "the opening epoch must be visible before any barrier closes")
	require.Equal(t, coord.RunID(), meta.RunID, "envelope RunID must mirror the coordinator")
	require.Equal(t, coord.EpochID(), meta.EpochID, "envelope epoch must mirror the coordinator")
}

// TestCoordinatorRunIDsArePerRun proves two runs never share a compile
// identity: epoch counters are per-run and therefore not comparable across
// runs, so the compiler cache must distinguish them by RunID.
func TestCoordinatorRunIDsArePerRun(t *testing.T) {
	envA := contextdata.NewEnvelope("task-a", "sess-1")
	ctxA := contextdata.WithEnvelope(context.Background(), envA)
	coordA := NewEpochCoordinator(ctxA, nil, nil)

	envB := contextdata.NewEnvelope("task-b", "sess-1")
	ctxB := contextdata.WithEnvelope(context.Background(), envB)
	coordB := NewEpochCoordinator(ctxB, nil, nil)

	require.NotEqual(t, coordA.RunID(), coordB.RunID(), "distinct runs must have distinct compile identities")
	require.Equal(t, uint64(1), envA.AssemblyMetadataSnapshot().EpochID)
	require.Equal(t, uint64(1), envB.AssemblyMetadataSnapshot().EpochID)
}

// TestCoordinatorWithoutEnvelopeIsScopeless proves the constructor is
// envelope-less-safe: no seed, no panic. Such compiles carry an empty RunID
// and share the scope-less namespace by construction (no coordinator means no
// barriers and no grounding).
func TestCoordinatorWithoutEnvelopeIsScopeless(t *testing.T) {
	coord := NewEpochCoordinator(context.Background(), nil, nil)
	require.NotPanics(t, func() { _ = coord.RunID() })
	require.NotEmpty(t, coord.RunID())
}

// TestEpochCloseAdvancesSeededIdentity proves the seeded EpochID advances with
// the epoch close, so post-barrier compiles miss while same-epoch compiles hit.
func TestEpochCloseAdvancesSeededIdentity(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "sess-1")
	ctx := contextdata.WithEnvelope(context.Background(), env)
	coord := NewEpochCoordinator(ctx, nil, nil)

	require.Equal(t, uint64(1), env.AssemblyMetadataSnapshot().EpochID)

	// FinalEpoch always closes (the run-end barrier); the per-node barrier is
	// conditional and no-ops on a clean epoch by design (D2).
	require.NoError(t, coord.FinalEpoch("node-1"))
	require.Equal(t, uint64(2), env.AssemblyMetadataSnapshot().EpochID)
	require.Equal(t, coord.RunID(), env.AssemblyMetadataSnapshot().RunID, "RunID survives the epoch close")
}

// TestCoordinatorAdoptsPreOwnedEnvelopeEpoch proves the pause/resume contract
// (ruling R-2): the DSL ask path re-enters a graph on a SURVIVING envelope, so
// a fresh coordinator must adopt the envelope's memory order — replacing only
// the RunID — instead of restarting the counter and rewinding the epoch. A
// restarted counter would re-stamp pre-pause epochs and the stale guard would
// drop every post-resume compile.
func TestCoordinatorAdoptsPreOwnedEnvelopeEpoch(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "sess-1")
	env.UpdateAssemblyMetadata(func(m contextdata.AssemblyMeta) contextdata.AssemblyMeta {
		m.EpochID = 5
		return m
	})
	ctx := contextdata.WithEnvelope(context.Background(), env)

	coord := NewEpochCoordinator(ctx, nil, nil)

	meta := env.AssemblyMetadataSnapshot()
	require.Equal(t, uint64(5), coord.EpochID(), "the counter continues from the adopted memory order")
	require.Equal(t, uint64(5), meta.EpochID, "adoption must not rewind the envelope")
	require.NotEmpty(t, meta.RunID)
	require.Equal(t, coord.RunID(), meta.RunID, "only the run identity is replaced")
}
