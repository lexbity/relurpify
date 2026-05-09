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

	IntentEvidenceKey       = ClarificationNamespace + ".evidence"
	IntentInterpretationKey = ClarificationNamespace + ".interpretation"
	RouteResolutionKey      = ClarificationNamespace + ".route_resolution"
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

// CanonicalWorkingMemoryKeys returns the clarification and route-state keys in
// a stable order for checkpointing and restoration.
func CanonicalWorkingMemoryKeys() []string {
	keys := append([]string(nil), ClarificationWorkingMemoryKeys()...)
	keys = append(keys, IntentEvidenceKey, IntentInterpretationKey, RouteResolutionKey)
	return keys
}
