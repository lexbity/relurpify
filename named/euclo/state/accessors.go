package state

import (
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/named/euclo/policy"
)

// --- Task and Intake ---

// GetTaskEnvelope retrieves the normalized task envelope.
func GetTaskEnvelope(env *contextdata.Envelope) (*intake.TaskEnvelope, bool) {
	return contextdata.GetTyped[*intake.TaskEnvelope](env, KeyTaskEnvelope)
}

// SetTaskEnvelope stores the normalized task envelope.
func SetTaskEnvelope(env *contextdata.Envelope, te *intake.TaskEnvelope) {
	contextdata.SetTyped(env, KeyTaskEnvelope, te)
}

// GetIntentClassification retrieves the classification result.
func GetIntentClassification(env *contextdata.Envelope) (*intake.IntentClassification, bool) {
	return contextdata.GetTyped[*intake.IntentClassification](env, KeyIntentClassification)
}

// SetIntentClassification stores the classification result.
func SetIntentClassification(env *contextdata.Envelope, ic *intake.IntentClassification) {
	contextdata.SetTyped(env, KeyIntentClassification, ic)
}

// GetIntentEvidence retrieves the structured evidence record.
func GetIntentEvidence(env *contextdata.Envelope) (*intentcontext.IntentEvidence, bool) {
	return contextdata.GetTyped[*intentcontext.IntentEvidence](env, KeyIntentEvidence)
}

// SetIntentEvidence stores the structured evidence record.
// Writes to both KeyIntentEvidence and the intentcontext canonical key so
// clarification state readers see the same value without a second write.
func SetIntentEvidence(env *contextdata.Envelope, evidence *intentcontext.IntentEvidence) {
	contextdata.SetTyped(env, KeyIntentEvidence, evidence)
	contextdata.SetTyped(env, intentcontext.IntentEvidenceKey, evidence)
}

// GetIntentInterpretation retrieves the structured interpretation record.
func GetIntentInterpretation(env *contextdata.Envelope) (*intentcontext.IntentInterpretation, bool) {
	return contextdata.GetTyped[*intentcontext.IntentInterpretation](env, KeyIntentInterpretation)
}

// SetIntentInterpretation stores the structured interpretation record.
// Writes to both KeyIntentInterpretation and the intentcontext canonical key.
func SetIntentInterpretation(env *contextdata.Envelope, interpretation *intentcontext.IntentInterpretation) {
	contextdata.SetTyped(env, KeyIntentInterpretation, interpretation)
	contextdata.SetTyped(env, intentcontext.IntentInterpretationKey, interpretation)
}

// GetRouteSelection retrieves the resolved route.
func GetRouteSelection(env *contextdata.Envelope) (*euclotypes.RouteSelection, bool) {
	return contextdata.GetTyped[*euclotypes.RouteSelection](env, KeyRouteSelection)
}

// SetRouteSelection stores the resolved route.
func SetRouteSelection(env *contextdata.Envelope, rs *euclotypes.RouteSelection) {
	contextdata.SetTyped(env, KeyRouteSelection, rs)
}

// GetRouteResolution retrieves the selected route resolution record.
func GetRouteResolution(env *contextdata.Envelope) (*euclotypes.RouteResolution, bool) {
	return contextdata.GetTyped[*euclotypes.RouteResolution](env, KeyRouteResolution)
}

// SetRouteResolution stores the selected route resolution record.
// Writes to both KeyRouteResolution and the intentcontext canonical key.
func SetRouteResolution(env *contextdata.Envelope, resolution *euclotypes.RouteResolution) {
	contextdata.SetTyped(env, KeyRouteResolution, resolution)
	contextdata.SetTyped(env, intentcontext.RouteResolutionKey, resolution)
}

// --- User Hints ---

// GetContextHint retrieves the context hint override.
func GetContextHint(env *contextdata.Envelope) (string, bool) {
	return contextdata.GetTyped[string](env, KeyContextHint)
}

// SetContextHint stores the context hint override.
func SetContextHint(env *contextdata.Envelope, hint string) {
	contextdata.SetTyped(env, KeyContextHint, hint)
}

// GetWorkspaceScopes retrieves the workspace scopes.
func GetWorkspaceScopes(env *contextdata.Envelope) ([]string, bool) {
	return contextdata.GetTyped[[]string](env, KeyWorkspaceScopes)
}

// SetWorkspaceScopes stores the workspace scopes.
func SetWorkspaceScopes(env *contextdata.Envelope, scopes []string) {
	contextdata.SetTyped(env, KeyWorkspaceScopes, scopes)
}

// GetSessionHint retrieves the session hint.
func GetSessionHint(env *contextdata.Envelope) (string, bool) {
	return contextdata.GetTyped[string](env, KeySessionHint)
}

// SetSessionHint stores the session hint.
func SetSessionHint(env *contextdata.Envelope, hint string) {
	contextdata.SetTyped(env, KeySessionHint, hint)
}

// GetFollowUpHint retrieves the follow-up hint.
func GetFollowUpHint(env *contextdata.Envelope) (string, bool) {
	return contextdata.GetTyped[string](env, KeyFollowUpHint)
}

// SetFollowUpHint stores the follow-up hint.
func SetFollowUpHint(env *contextdata.Envelope, hint string) {
	contextdata.SetTyped(env, KeyFollowUpHint, hint)
}

// GetAgentModeHint retrieves the agent mode hint.
func GetAgentModeHint(env *contextdata.Envelope) (string, bool) {
	return contextdata.GetTyped[string](env, KeyAgentModeHint)
}

// SetAgentModeHint stores the agent mode hint.
func SetAgentModeHint(env *contextdata.Envelope, hint string) {
	contextdata.SetTyped(env, KeyAgentModeHint, hint)
}

// GetString retrieves a string value from the envelope working memory.
func GetString(env *contextdata.Envelope, key string) string {
	v, ok := contextdata.GetTyped[string](env, key)
	if !ok {
		return ""
	}
	return v
}

// --- Ingestion ---

// GetUserSelectedFiles retrieves the user-selected files.
func GetUserSelectedFiles(env *contextdata.Envelope) ([]string, bool) {
	return contextdata.GetTyped[[]string](env, KeyUserSelectedFiles)
}

// SetUserSelectedFiles stores the user-selected files.
func SetUserSelectedFiles(env *contextdata.Envelope, files []string) {
	contextdata.SetTyped(env, KeyUserSelectedFiles, files)
}

// GetExplicitIngestPaths retrieves explicit ingest paths.
func GetExplicitIngestPaths(env *contextdata.Envelope) ([]string, bool) {
	return contextdata.GetTyped[[]string](env, KeyExplicitIngestPaths)
}

// SetExplicitIngestPaths stores explicit ingest paths.
func SetExplicitIngestPaths(env *contextdata.Envelope, paths []string) {
	contextdata.SetTyped(env, KeyExplicitIngestPaths, paths)
}

// GetIncrementalSinceRef retrieves the incremental since ref.
func GetIncrementalSinceRef(env *contextdata.Envelope) (string, bool) {
	return contextdata.GetTyped[string](env, KeyIncrementalSinceRef)
}

// SetIncrementalSinceRef stores the incremental since ref.
func SetIncrementalSinceRef(env *contextdata.Envelope, ref string) {
	contextdata.SetTyped(env, KeyIncrementalSinceRef, ref)
}

// GetIngestPolicy retrieves the ingest policy.
func GetIngestPolicy(env *contextdata.Envelope) (string, bool) {
	return contextdata.GetTyped[string](env, KeyIngestPolicy)
}

// SetIngestPolicy stores the ingest policy.
func SetIngestPolicy(env *contextdata.Envelope, policy string) {
	contextdata.SetTyped(env, KeyIngestPolicy, policy)
}

// --- Intent Signals ---

// GetIntentSignals retrieves family scores from classification.
func GetIntentSignals(env *contextdata.Envelope) (map[string]float64, bool) {
	return contextdata.GetTyped[map[string]float64](env, KeyIntentSignals)
}

// SetIntentSignals stores family scores from classification.
func SetIntentSignals(env *contextdata.Envelope, scores map[string]float64) {
	contextdata.SetTyped(env, KeyIntentSignals, scores)
}

// SetThoughtRecipeID stores the thoughtrecipe ID.
func SetThoughtRecipeID(env *contextdata.Envelope, id string) {
	contextdata.SetTyped(env, KeyThoughtRecipeID, id)
}

// --- Thought ThoughtRecipe ---

// GetThoughtRecipeID retrieves the thoughtrecipe ID.
func GetThoughtRecipeID(env *contextdata.Envelope) (string, bool) {
	return contextdata.GetTyped[string](env, KeyThoughtRecipeID)
}

// GetThoughtRecipeVersion retrieves the thoughtrecipe version.
func GetThoughtRecipeVersion(env *contextdata.Envelope) (string, bool) {
	return contextdata.GetTyped[string](env, KeyThoughtRecipeVersion)
}

// SetThoughtRecipeVersion stores the thoughtrecipe version.
func SetThoughtRecipeVersion(env *contextdata.Envelope, version string) {
	contextdata.SetTyped(env, KeyThoughtRecipeVersion, version)
}

// --- Policy ---

// GetHITLTriggered retrieves whether HITL was triggered.
func GetHITLTriggered(env *contextdata.Envelope) (bool, bool) {
	return contextdata.GetTyped[bool](env, KeyHITLTriggered)
}

// GetHITLResponse retrieves the HITL response.
func GetHITLResponse(env *contextdata.Envelope) (*interaction.HITLResponse, bool) {
	return contextdata.GetTyped[*interaction.HITLResponse](env, KeyHITLResponse)
}

// SetHITLResponse stores the HITL response.
func SetHITLResponse(env *contextdata.Envelope, resp *interaction.HITLResponse) {
	contextdata.SetTyped(env, KeyHITLResponse, resp)
}

// SetHITLTriggered stores whether HITL was triggered.
func SetHITLTriggered(env *contextdata.Envelope, triggered bool) {
	contextdata.SetTyped(env, KeyHITLTriggered, triggered)
}

// GetPolicyDecision retrieves the policy decision.
func GetPolicyDecision(env *contextdata.Envelope) (*policy.PolicyDecision, bool) {
	return contextdata.GetTyped[*policy.PolicyDecision](env, KeyPolicyDecision)
}

// SetPolicyDecision stores the policy decision.
func SetPolicyDecision(env *contextdata.Envelope, pd *policy.PolicyDecision) {
	contextdata.SetTyped(env, KeyPolicyDecision, pd)
}

// --- Execution ---

// GetDryRunMode retrieves dry run mode.
func GetDryRunMode(env *contextdata.Envelope) (bool, bool) {
	return contextdata.GetTyped[bool](env, KeyDryRunMode)
}

// SetDryRunMode stores dry run mode.
func SetDryRunMode(env *contextdata.Envelope, dryRun bool) {
	contextdata.SetTyped(env, KeyDryRunMode, dryRun)
}

// GetOutcomeCategory retrieves the outcome category.
func GetOutcomeCategory(env *contextdata.Envelope) (string, bool) {
	return contextdata.GetTyped[string](env, KeyOutcomeCategory)
}

// SetOutcomeCategory stores the outcome category.
func SetOutcomeCategory(env *contextdata.Envelope, category string) {
	contextdata.SetTyped(env, KeyOutcomeCategory, category)
}

// GetOutcomeArtifacts retrieves outcome artifacts.
func GetOutcomeArtifacts(env *contextdata.Envelope) ([]string, bool) {
	return contextdata.GetTyped[[]string](env, KeyOutcomeArtifacts)
}

// SetOutcomeArtifacts stores outcome artifacts.
func SetOutcomeArtifacts(env *contextdata.Envelope, artifacts []string) {
	contextdata.SetTyped(env, KeyOutcomeArtifacts, artifacts)
}

// GetOutcomeTelemetry retrieves outcome telemetry.
func GetOutcomeTelemetry(env *contextdata.Envelope) (map[string]any, bool) {
	return contextdata.GetTyped[map[string]any](env, KeyOutcomeTelemetry)
}

// SetOutcomeTelemetry stores outcome telemetry.
func SetOutcomeTelemetry(env *contextdata.Envelope, telemetry map[string]any) {
	contextdata.SetTyped(env, KeyOutcomeTelemetry, telemetry)
}

// --- Resume (Session Restoration) ---

// GetResumeClassification retrieves the resume classification.
func GetResumeClassification(env *contextdata.Envelope) (*intake.IntentClassification, bool) {
	return contextdata.GetTyped[*intake.IntentClassification](env, KeyResumeClassification)
}

// SetResumeClassification stores the resume classification.
func SetResumeClassification(env *contextdata.Envelope, ic *intake.IntentClassification) {
	contextdata.SetTyped(env, KeyResumeClassification, ic)
}

// GetResumeRoute retrieves the resume route.
func GetResumeRoute(env *contextdata.Envelope) (*euclotypes.RouteSelection, bool) {
	return contextdata.GetTyped[*euclotypes.RouteSelection](env, KeyResumeRoute)
}

// SetResumeRoute stores the resume route.
func SetResumeRoute(env *contextdata.Envelope, rs *euclotypes.RouteSelection) {
	contextdata.SetTyped(env, KeyResumeRoute, rs)
}

// --- Stream ---

// GetStreamTokenUsage retrieves stream token usage.
func GetStreamTokenUsage(env *contextdata.Envelope) (int, bool) {
	return contextdata.GetTyped[int](env, KeyStreamTokenUsage)
}

// SetStreamTokenUsage stores stream token usage.
func SetStreamTokenUsage(env *contextdata.Envelope, usage int) {
	contextdata.SetTyped(env, KeyStreamTokenUsage, usage)
}

// GetStreamResult retrieves the context stream result produced at intake.
func GetStreamResult(env *contextdata.Envelope) (*contextstream.Result, bool) {
	return contextdata.GetTyped[*contextstream.Result](env, KeyStreamResult)
}

// SetStreamResult stores the context stream result produced at intake.
func SetStreamResult(env *contextdata.Envelope, result *contextstream.Result) {
	contextdata.SetTyped(env, KeyStreamResult, result)
}

// GetDispatchRouteKind retrieves the dispatch route kind.
func GetDispatchRouteKind(env *contextdata.Envelope) (string, bool) {
	return contextdata.GetTyped[string](env, KeyDispatchRouteKind)
}

// SetDispatchRouteKind stores the dispatch route kind.
func SetDispatchRouteKind(env *contextdata.Envelope, kind string) {
	contextdata.SetTyped(env, KeyDispatchRouteKind, kind)
}

// --- Frame History ---

// GetFrameHistory retrieves the frame history.
func GetFrameHistory(env *contextdata.Envelope) ([]string, bool) {
	return contextdata.GetTyped[[]string](env, KeyFrameHistory)
}

// SetFrameHistory stores the frame history.
func SetFrameHistory(env *contextdata.Envelope, frames []string) {
	contextdata.SetTyped(env, KeyFrameHistory, frames)
}

// AppendFrameID appends a frame ID to the history.
func AppendFrameID(env *contextdata.Envelope, frameID string) {
	history, _ := GetFrameHistory(env)
	history = append(history, frameID)
	SetFrameHistory(env, history)
}

// --- Job Records ---

// GetJobRecords retrieves the job history.
func GetJobRecords(env *contextdata.Envelope) ([]JobRecord, bool) {
	return contextdata.GetTyped[[]JobRecord](env, KeyJobRecords)
}

// SetJobRecords stores the job history.
func SetJobRecords(env *contextdata.Envelope, records []JobRecord) {
	contextdata.SetTyped(env, KeyJobRecords, records)
}

// AppendJobRecord appends a job record to the history.
func AppendJobRecord(env *contextdata.Envelope, record JobRecord) {
	records, _ := GetJobRecords(env)
	records = append(records, record)
	SetJobRecords(env, records)
}

// --- Negative Constraints ---

// GetNegativeConstraints retrieves the negative constraint seeds.
func GetNegativeConstraints(env *contextdata.Envelope) ([]string, bool) {
	return contextdata.GetTyped[[]string](env, KeyNegativeConstraints)
}

// SetNegativeConstraints stores the negative constraint seeds.
func SetNegativeConstraints(env *contextdata.Envelope, constraints []string) {
	contextdata.SetTyped(env, KeyNegativeConstraints, constraints)
}

// --- Family Selection (Resume) ---

// GetFamilySelection retrieves the selected family.
func GetFamilySelection(env *contextdata.Envelope) (string, bool) {
	return contextdata.GetTyped[string](env, KeyFamilySelection)
}

// SetFamilySelection stores the selected family.
func SetFamilySelection(env *contextdata.Envelope, family string) {
	contextdata.SetTyped(env, KeyFamilySelection, family)
}
