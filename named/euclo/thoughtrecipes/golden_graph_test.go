package thoughtrecipe

import (
	"encoding/json"
	"os"
	"sort"
	"testing"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
)

func TestGoldenGraphs(t *testing.T) {
	recipes := loadGoldenRecipes(t)
	for name, doc := range recipes {
		t.Run(name, func(t *testing.T) {
			plan, err := LowerDocument(doc)
			if err != nil {
				t.Fatalf("LowerDocument(%s) failed: %v", name, err)
			}
			graph, err := BuildThoughtRecipeGraph(plan, &paradigm.Deps{}, nil)
			if err != nil {
				t.Fatalf("BuildThoughtRecipeGraph(%s) failed: %v", name, err)
			}
			if err := graph.Validate(); err != nil {
				t.Fatalf("graph validation failed for %s: %v", name, err)
			}
			goldenPath := goldenPath(t, name, "graph.json")
			got := serializeGraphSnapshot(t, graph)

			if update := os.Getenv("UPDATE_GOLDEN"); update != "" {
				if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
					t.Fatalf("write golden %s: %v", goldenPath, err)
				}
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden %s: %v (set UPDATE_GOLDEN=1 to create)", goldenPath, err)
			}
			if string(got) != string(want) {
				t.Fatalf("golden graph mismatch for %s\ngot:\n%s\nwant:\n%s\n(set UPDATE_GOLDEN=1 to re-baseline)",
					name, string(got), string(want))
			}
		})
	}
}

func TestGoldenGraphsFailOnMutation(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe mutate_graph
"Mutation test."

trigger as capability:
  may read workspace

agent reviewer uses react

run reviewer:
  goal "Original goal."
`)
	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}
	originalID := plan.Steps[0].ID

	plan.Steps[0].ID = "mutated." + originalID
	graph, err := BuildThoughtRecipeGraph(plan, &paradigm.Deps{}, nil)
	if err != nil {
		t.Fatalf("BuildThoughtRecipeGraph failed: %v", err)
	}
	snap := serializeGraphSnapshot(t, graph)
	if containsNodeID(t, graph, originalID+".execute") {
		t.Fatal("mutation test: original node ID should NOT be present in mutated graph")
	}
	_ = snap
}

func containsNodeID(t *testing.T, g *agentgraph.Graph, id string) bool {
	t.Helper()
	for _, nid := range g.NodeIDs() {
		if nid == id {
			return true
		}
	}
	return false
}

type graphSnapshot struct {
	StartNode string          `json:"start_node"`
	Nodes     []graphNodeInfo `json:"nodes"`
	Edges     []graphEdgeInfo `json:"edges"`
}

type graphNodeInfo struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type graphEdgeInfo struct {
	From         string `json:"from"`
	To           string `json:"to"`
	HasCondition bool   `json:"has_condition"`
	Parallel     bool   `json:"parallel"`
}

func serializeGraphSnapshot(t *testing.T, g *agentgraph.Graph) []byte {
	t.Helper()
	nodeIDs := g.NodeIDs()
	nodes := make([]graphNodeInfo, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		nt, ok := g.NodeType(id)
		if !ok {
			t.Fatalf("node %s has no type", id)
		}
		nodes = append(nodes, graphNodeInfo{ID: id, Type: string(nt)})
	}

	edges := make([]graphEdgeInfo, 0)
	for _, id := range nodeIDs {
		for _, edge := range g.OutgoingEdges(id) {
			edges = append(edges, graphEdgeInfo{
				From:         edge.From,
				To:           edge.To,
				HasCondition: edge.Condition != nil,
				Parallel:     edge.Parallel,
			})
		}
	}

	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})

	snap := graphSnapshot{
		StartNode: g.StartNodeID(),
		Nodes:     nodes,
		Edges:     edges,
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatalf("json marshal graph: %v", err)
	}
	return data
}
