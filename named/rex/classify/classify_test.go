package classify

import (
	"testing"

	"github.com/lexcodex/relurpify/named/rex/envelope"
)

func TestClassifyDetectsDeterministicMutation(t *testing.T) {
	class := Classify(envelope.Envelope{
		Instruction:   "implement a structured pipeline fix and resume after checkpoint",
		EditPermitted: true,
	})
	if !class.MutationCapable || !class.DeterministicPreferred || !class.RecoveryHeavy {
		t.Fatalf("unexpected classification: %+v", class)
	}
}

func TestClassifyCoversReadOnlyReviewAndManagedPaths(t *testing.T) {
	review := Classify(envelope.Envelope{
		Instruction:   "review the findings and audit the results",
		EditPermitted: false,
	})
	if !review.ReadOnly || review.MutationCapable || review.Intent != "review" || review.RiskLevel != "low" {
		t.Fatalf("unexpected review classification: %+v", review)
	}
	mutation := Classify(envelope.Envelope{
		Instruction:   "plan and implement the patch for the background loop",
		EditPermitted: true,
		WorkflowID:    "wf-1",
	})
	if !mutation.LongRunningManaged || !mutation.MutationCapable || mutation.Intent != "mutation" || mutation.RiskLevel != "high" {
		t.Fatalf("unexpected mutation classification: %+v", mutation)
	}
	if len(mutation.ReasonCodes) == 0 {
		t.Fatalf("expected reason codes: %+v", mutation)
	}
}
