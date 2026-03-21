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
