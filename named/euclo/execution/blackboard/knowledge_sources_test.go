package blackboard

import (
	"strings"
	"testing"

	agentblackboard "github.com/lexcodex/relurpify/agents/blackboard"
)

func TestApplyKnowledgeSourceResponseAcceptsFencedJSON(t *testing.T) {
	bb := agentblackboard.NewBlackboard()
	raw := "```json\n{\"facts\":[{\"key\":\"archaeology:patterns\",\"value\":[\"normalize\"]}]}\n```"

	if err := applyKnowledgeSourceResponse(bb, "Pattern Mapper", raw); err != nil {
		t.Fatalf("applyKnowledgeSourceResponse returned error: %v", err)
	}
	if len(bb.Facts) != 1 {
		t.Fatalf("expected one blackboard fact, got %#v", bb.Facts)
	}
	if bb.Facts[0].Key != "archaeology:patterns" {
		t.Fatalf("unexpected fact key: %#v", bb.Facts[0])
	}
	if !strings.Contains(bb.Facts[0].Value, "normalize") {
		t.Fatalf("unexpected recorded fact: %#v", bb.Facts[0].Value)
	}
}
