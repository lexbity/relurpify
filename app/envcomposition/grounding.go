package envcomposition

import (
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
)

// WireGrounding is the single composition of the knowledge write boundary
// (D-5): the grounding service becomes the graph's epoch grounder and the
// invalidation drain the barrier's subscriber-side drain. Production, the
// live harness, and the dry run all call this — the era of a harness wiring
// half the runtime ends here. A nil knowledge runtime returns deps unchanged:
// callers that legitimately run without knowledge pass nil.
func WireGrounding(deps *paradigm.Deps, kr *KnowledgeRuntime) *paradigm.Deps {
	if deps == nil || kr == nil {
		return deps
	}
	deps.Grounder = kr.Grounding
	deps.EpochDrain = kr.Drain
	return deps
}
