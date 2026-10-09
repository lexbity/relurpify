package agentgraph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// TestSealedGraph verifies the two-phase lifecycle: build-phase mutators
// succeed before Execute, every mutator returns ErrGraphSealed afterwards, and
// Execute itself remains runnable on a sealed graph (sealing governs mutation,
// not execution).
func TestSealedGraph(t *testing.T) {
	g := NewGraph()
	require.NoError(t, g.AddNode(NewTerminalNode("done")))
	require.NoError(t, g.SetStart("done"))

	// Build phase: all mutators are legal.
	require.NoError(t, g.SetMaxNodeVisits(7))
	require.NoError(t, g.SetTelemetry(nil))
	require.NoError(t, g.SetCapabilityCatalog(nil))

	env := contextdata.NewEnvelope("task-seal", "session")
	_, err := g.Execute(context.Background(), env)
	require.NoError(t, err)

	cases := []struct {
		name string
		call func() error
	}{
		{"AddNode", func() error { return g.AddNode(NewTerminalNode("extra")) }},
		{"AddEdge", func() error { return g.AddEdge("done", "done", nil, false) }},
		{"SetStart", func() error { return g.SetStart("done") }},
		{"SetTelemetry", func() error { return g.SetTelemetry(nil) }},
		{"SetMaxNodeVisits", func() error { return g.SetMaxNodeVisits(3) }},
		{"SetCapabilityCatalog", func() error { return g.SetCapabilityCatalog(nil) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorIs(t, tc.call(), ErrGraphSealed)
		})
	}

	// A sealed graph may still be executed again.
	_, err = g.Execute(context.Background(), env)
	require.NoError(t, err)
}
