package state

import (
	"fmt"
)

// JobRecord represents a single job entry in the job history.
type JobRecord struct {
	ID        string
	Type      string
	Status    string
	Timestamp int64
	Metadata  map[string]any
}

// RecipeCaptureKey constructs the working memory key for a recipe capture.
// Format: euclo.recipe.{recipeID}.{captureName}
func RecipeCaptureKey(recipeID, captureName string) string {
	return fmt.Sprintf("%s%s.%s", KeyRecipePrefix, recipeID, captureName)
}

// IngestionPolicy values for KeyIngestPolicy.
const (
	IngestPolicyFilesOnly   = "files_only"
	IngestPolicyIncremental = "incremental"
	IngestPolicyFull        = "full"
)

// OutcomeCategory values for KeyOutcomeCategory.
const (
	OutcomeSuccess        = "success"
	OutcomePartialSuccess = "partial_success"
	OutcomeFailure        = "failure"
	OutcomeHITL           = "hitl"
	OutcomeDeferred       = "deferred"
)
