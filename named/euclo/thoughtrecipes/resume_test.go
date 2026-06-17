package thoughtrecipe

import (
	"testing"
)

// TestResume_GraphIdentity verifies that compiling a recipe twice yields
// identical graph topology including node IDs (FR-9).
// This is the guard against non-deterministic node-ID generation that
// would break pause/resume contract.
func TestResume_GraphIdentity(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe resume_identity
"Resume identity test."

trigger as capability:
  may read workspace

agent reviewer uses react

run reviewer:
  goal "Step one."

ask user:
  question "Continue?"
  choices ["yes", "no"]

run reviewer:
  goal "Step two."
`)

	// Compile twice.
	plan1, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("first LowerDocument failed: %v", err)
	}
	plan2, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("second LowerDocument failed: %v", err)
	}

	// Plans must be identical.
	if len(plan1.Steps) != len(plan2.Steps) {
		t.Fatalf("step count mismatch: %d vs %d", len(plan1.Steps), len(plan2.Steps))
	}
	for i := range plan1.Steps {
		if plan1.Steps[i].ID != plan2.Steps[i].ID {
			t.Fatalf("step %d ID mismatch: %q vs %q", i, plan1.Steps[i].ID, plan2.Steps[i].ID)
		}
	}

	// Graph topology must be identical.
	graph1, err := BuildThoughtRecipeGraph(plan1, nil, nil)
	if err != nil {
		t.Fatalf("first BuildThoughtRecipeGraph failed: %v", err)
	}
	graph2, err := BuildThoughtRecipeGraph(plan2, nil, nil)
	if err != nil {
		t.Fatalf("second BuildThoughtRecipeGraph failed: %v", err)
	}

	nodeIDs1 := graph1.NodeIDs()
	nodeIDs2 := graph2.NodeIDs()
	if len(nodeIDs1) != len(nodeIDs2) {
		t.Fatalf("node count mismatch: %d vs %d", len(nodeIDs1), len(nodeIDs2))
	}
	for i := range nodeIDs1 {
		if nodeIDs1[i] != nodeIDs2[i] {
			t.Fatalf("node ID %d mismatch: %q vs %q", i, nodeIDs1[i], nodeIDs2[i])
		}
	}

	// Edge count must match.
	var edgeCount1 int
	for _, id := range nodeIDs1 {
		edgeCount1 += len(graph1.OutgoingEdges(id))
	}
	var edgeCount2 int
	for _, id := range nodeIDs2 {
		edgeCount2 += len(graph2.OutgoingEdges(id))
	}
	if edgeCount1 != edgeCount2 {
		t.Fatalf("edge count mismatch: %d vs %d", edgeCount1, edgeCount2)
	}
}
