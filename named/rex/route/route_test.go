package route

import (
	"testing"

	"github.com/lexcodex/relurpify/named/rex/classify"
	"github.com/lexcodex/relurpify/named/rex/envelope"
)

func TestDecideSelectsPipelineForDeterministicTasks(t *testing.T) {
	decision := Decide(envelope.Envelope{}, classify.Classification{DeterministicPreferred: true})
	if decision.Family != FamilyPipeline {
		t.Fatalf("Family = %q", decision.Family)
	}
}

func TestDecideSelectsArchitectForMutationRecovery(t *testing.T) {
	decision := Decide(envelope.Envelope{}, classify.Classification{MutationCapable: true, RecoveryHeavy: true})
	if decision.Family != FamilyArchitect {
		t.Fatalf("Family = %q", decision.Family)
	}
}
