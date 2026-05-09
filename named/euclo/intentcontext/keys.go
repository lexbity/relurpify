package intentcontext

// Canonical working-memory keys for clarification state.
const (
	ClarificationNamespace = "euclo.intent.clarification"

	ClarificationStateKey               = ClarificationNamespace + ".state"
	ClarificationAmbiguityKey           = ClarificationNamespace + ".ambiguity"
	ClarificationTurnsKey               = ClarificationNamespace + ".turns"
	ClarificationConfirmedEntitiesKey   = ClarificationNamespace + ".confirmed_entities"
	ClarificationConfirmedScopesKey     = ClarificationNamespace + ".confirmed_scopes"
	ClarificationRelationIntentsKey     = ClarificationNamespace + ".relation_intents"
	ClarificationGroundedAnchorsKey     = ClarificationNamespace + ".grounded_anchors"
	ClarificationPendingProjectionKey   = ClarificationNamespace + ".pending_projection"
	ClarificationProjectedMutationsKey  = ClarificationNamespace + ".projected_mutations"
	ClarificationActiveThoughtRecipeKey = ClarificationNamespace + ".active_thoughtrecipe"
	ClarificationLastCheckpointIDKey    = ClarificationNamespace + ".last_checkpoint_id"
	ClarificationLastCheckpointSeqKey   = ClarificationNamespace + ".last_checkpoint_seq"
)

// ClarificationWorkingMemoryKeys returns the canonical clarification keys in
// write order so they can be checkpointed and restored together.
func ClarificationWorkingMemoryKeys() []string {
	return []string{
		ClarificationStateKey,
		ClarificationAmbiguityKey,
		ClarificationTurnsKey,
		ClarificationConfirmedEntitiesKey,
		ClarificationConfirmedScopesKey,
		ClarificationRelationIntentsKey,
		ClarificationGroundedAnchorsKey,
		ClarificationPendingProjectionKey,
		ClarificationProjectedMutationsKey,
		ClarificationActiveThoughtRecipeKey,
		ClarificationLastCheckpointIDKey,
		ClarificationLastCheckpointSeqKey,
	}
}
