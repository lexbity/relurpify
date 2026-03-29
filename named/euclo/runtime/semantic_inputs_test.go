package runtime

import (
	"testing"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
)

func TestSemanticInputBundleFromSources(t *testing.T) {
	bundle := SemanticInputBundleFromSources(
		"wf-1",
		&archaeodomain.VersionedLivingPlan{
			WorkflowID:              "wf-1",
			DerivedFromExploration:  "explore-1",
			BasedOnRevision:         "rev-1",
			SemanticSnapshotRef:     "snapshot-1",
			PatternRefs:             []string{"pattern-a"},
			TensionRefs:             []string{"tension-a"},
			FormationProvenanceRefs: []string{"prov-a"},
		},
		&SemanticRequestHistory{
			Requests: []archaeodomain.RequestRecord{
				{ID: "req-pattern", Kind: archaeodomain.RequestPatternSurfacing, Status: archaeodomain.RequestStatusPending, SubjectRefs: []string{"sym:/pkg/foo.go"}},
				{ID: "req-prospective", Kind: archaeodomain.RequestProspectiveAnalysis, Status: archaeodomain.RequestStatusCompleted},
			},
		},
		&SemanticProvenance{
			ConvergenceRefs: []string{"conv-1"},
			Learning: []SemanticLearningRef{
				{InteractionID: "learn-1"},
			},
		},
		&SemanticLearningQueue{
			PendingLearning: []SemanticLearningRef{{InteractionID: "learn-2"}},
		},
		&archaeodomain.WorkspaceConvergenceProjection{
			Current: &archaeodomain.ConvergenceRecord{ID: "conv-current"},
		},
	)
	if bundle.WorkflowID != "wf-1" {
		t.Fatalf("unexpected workflow id: %#v", bundle)
	}
	if len(bundle.PatternRefs) == 0 || bundle.PatternRefs[0] != "pattern-a" {
		t.Fatalf("unexpected pattern refs: %#v", bundle.PatternRefs)
	}
	if len(bundle.PatternFindings) == 0 {
		t.Fatalf("expected pattern findings: %#v", bundle)
	}
	if len(bundle.PendingRequests) != 1 || bundle.PendingRequests[0].RequestID != "req-pattern" {
		t.Fatalf("unexpected pending requests: %#v", bundle.PendingRequests)
	}
	if len(bundle.ProspectiveRefs) != 1 || bundle.ProspectiveRefs[0] != "req-prospective" {
		t.Fatalf("unexpected prospective refs: %#v", bundle.ProspectiveRefs)
	}
	if len(bundle.ProspectiveFindings) != 1 || bundle.ProspectiveFindings[0].RefID != "req-prospective" {
		t.Fatalf("unexpected prospective findings: %#v", bundle.ProspectiveFindings)
	}
	if len(bundle.ConvergenceRefs) < 2 {
		t.Fatalf("expected convergence refs from provenance and current projection: %#v", bundle.ConvergenceRefs)
	}
	if len(bundle.ConvergenceFindings) < 1 {
		t.Fatalf("expected convergence findings: %#v", bundle.ConvergenceFindings)
	}
	if len(bundle.LearningInteractionRefs) != 2 {
		t.Fatalf("unexpected learning refs: %#v", bundle.LearningInteractionRefs)
	}
	if len(bundle.LearningFindings) != 2 {
		t.Fatalf("unexpected learning findings: %#v", bundle.LearningFindings)
	}
}
