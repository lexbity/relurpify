package state

import (
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	"codeburg.org/lexbit/relurpify/named/euclo/policy"
)

// --- Task and Intake ---

// GetTaskEnvelope retrieves the normalized task envelope.
func GetTaskEnvelope(env *contextdata.Envelope) (*intake.TaskEnvelope, bool) {
	v, ok := env.GetWorkingValue(KeyTaskEnvelope)
	if !ok {
		return nil, false
	}
	te, ok := v.(*intake.TaskEnvelope)
	return te, ok
}

// SetTaskEnvelope stores the normalized task envelope.
func SetTaskEnvelope(env *contextdata.Envelope, te *intake.TaskEnvelope) {
	env.SetWorkingValue(KeyTaskEnvelope, te, contextdata.MemoryClassTask)
}

// GetIntentClassification retrieves the classification result.
func GetIntentClassification(env *contextdata.Envelope) (*intake.IntentClassification, bool) {
	v, ok := env.GetWorkingValue(KeyIntentClassification)
	if !ok {
		return nil, false
	}
	ic, ok := v.(*intake.IntentClassification)
	return ic, ok
}

// SetIntentClassification stores the classification result.
func SetIntentClassification(env *contextdata.Envelope, ic *intake.IntentClassification) {
	env.SetWorkingValue(KeyIntentClassification, ic, contextdata.MemoryClassTask)
}

// GetIntentEvidence retrieves the structured evidence record.
func GetIntentEvidence(env *contextdata.Envelope) (*intentcontext.IntentEvidence, bool) {
	if env == nil {
		return nil, false
	}
	v, ok := env.GetWorkingValue(KeyIntentEvidence)
	if !ok {
		return nil, false
	}
	evidence, ok := v.(*intentcontext.IntentEvidence)
	return evidence, ok
}

// SetIntentEvidence stores the structured evidence record.
func SetIntentEvidence(env *contextdata.Envelope, evidence *intentcontext.IntentEvidence) {
	env.SetWorkingValue(KeyIntentEvidence, evidence, contextdata.MemoryClassTask)
}

// GetIntentInterpretation retrieves the structured interpretation record.
func GetIntentInterpretation(env *contextdata.Envelope) (*intentcontext.IntentInterpretation, bool) {
	if env == nil {
		return nil, false
	}
	v, ok := env.GetWorkingValue(KeyIntentInterpretation)
	if !ok {
		return nil, false
	}
	interpretation, ok := v.(*intentcontext.IntentInterpretation)
	return interpretation, ok
}

// SetIntentInterpretation stores the structured interpretation record.
func SetIntentInterpretation(env *contextdata.Envelope, interpretation *intentcontext.IntentInterpretation) {
	env.SetWorkingValue(KeyIntentInterpretation, interpretation, contextdata.MemoryClassTask)
}

// GetRouteSelection retrieves the resolved route.
func GetRouteSelection(env *contextdata.Envelope) (*orchestrate.RouteSelection, bool) {
	v, ok := env.GetWorkingValue(KeyRouteSelection)
	if !ok {
		return nil, false
	}
	rs, ok := v.(*orchestrate.RouteSelection)
	return rs, ok
}

// SetRouteSelection stores the resolved route.
func SetRouteSelection(env *contextdata.Envelope, rs *orchestrate.RouteSelection) {
	env.SetWorkingValue(KeyRouteSelection, rs, contextdata.MemoryClassTask)
}

// GetRouteResolution retrieves the selected route resolution record.
func GetRouteResolution(env *contextdata.Envelope) (*orchestrate.RouteResolution, bool) {
	if env == nil {
		return nil, false
	}
	v, ok := env.GetWorkingValue(KeyRouteResolution)
	if !ok {
		return nil, false
	}
	resolution, ok := v.(*orchestrate.RouteResolution)
	return resolution, ok
}

// SetRouteResolution stores the selected route resolution record.
func SetRouteResolution(env *contextdata.Envelope, resolution *orchestrate.RouteResolution) {
	env.SetWorkingValue(KeyRouteResolution, resolution, contextdata.MemoryClassTask)
}

// --- User Hints ---

// GetContextHint retrieves the context hint override.
func GetContextHint(env *contextdata.Envelope) (string, bool) {
	v, ok := env.GetWorkingValue(KeyContextHint)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// SetContextHint stores the context hint override.
func SetContextHint(env *contextdata.Envelope, hint string) {
	env.SetWorkingValue(KeyContextHint, hint, contextdata.MemoryClassTask)
}

// GetWorkspaceScopes retrieves the workspace scopes.
func GetWorkspaceScopes(env *contextdata.Envelope) ([]string, bool) {
	v, ok := env.GetWorkingValue(KeyWorkspaceScopes)
	if !ok {
		return nil, false
	}
	s, ok := v.([]string)
	return s, ok
}

// SetWorkspaceScopes stores the workspace scopes.
func SetWorkspaceScopes(env *contextdata.Envelope, scopes []string) {
	env.SetWorkingValue(KeyWorkspaceScopes, scopes, contextdata.MemoryClassTask)
}

// GetSessionHint retrieves the session hint.
func GetSessionHint(env *contextdata.Envelope) (string, bool) {
	v, ok := env.GetWorkingValue(KeySessionHint)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// SetSessionHint stores the session hint.
func SetSessionHint(env *contextdata.Envelope, hint string) {
	env.SetWorkingValue(KeySessionHint, hint, contextdata.MemoryClassTask)
}

// GetFollowUpHint retrieves the follow-up hint.
func GetFollowUpHint(env *contextdata.Envelope) (string, bool) {
	v, ok := env.GetWorkingValue(KeyFollowUpHint)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// SetFollowUpHint stores the follow-up hint.
func SetFollowUpHint(env *contextdata.Envelope, hint string) {
	env.SetWorkingValue(KeyFollowUpHint, hint, contextdata.MemoryClassTask)
}

// GetAgentModeHint retrieves the agent mode hint.
func GetAgentModeHint(env *contextdata.Envelope) (string, bool) {
	v, ok := env.GetWorkingValue(KeyAgentModeHint)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// SetAgentModeHint stores the agent mode hint.
func SetAgentModeHint(env *contextdata.Envelope, hint string) {
	env.SetWorkingValue(KeyAgentModeHint, hint, contextdata.MemoryClassTask)
}

// GetString retrieves a string value from the envelope working memory.
func GetString(env *contextdata.Envelope, key string) string {
	if env == nil {
		return ""
	}
	v, ok := env.GetWorkingValue(key)
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// --- Ingestion ---

// GetUserSelectedFiles retrieves the user-selected files.
func GetUserSelectedFiles(env *contextdata.Envelope) ([]string, bool) {
	v, ok := env.GetWorkingValue(KeyUserSelectedFiles)
	if !ok {
		return nil, false
	}
	s, ok := v.([]string)
	return s, ok
}

// SetUserSelectedFiles stores the user-selected files.
func SetUserSelectedFiles(env *contextdata.Envelope, files []string) {
	env.SetWorkingValue(KeyUserSelectedFiles, files, contextdata.MemoryClassTask)
}

// GetExplicitIngestPaths retrieves explicit ingest paths.
func GetExplicitIngestPaths(env *contextdata.Envelope) ([]string, bool) {
	v, ok := env.GetWorkingValue(KeyExplicitIngestPaths)
	if !ok {
		return nil, false
	}
	s, ok := v.([]string)
	return s, ok
}

// SetExplicitIngestPaths stores explicit ingest paths.
func SetExplicitIngestPaths(env *contextdata.Envelope, paths []string) {
	env.SetWorkingValue(KeyExplicitIngestPaths, paths, contextdata.MemoryClassTask)
}

// GetIncrementalSinceRef retrieves the incremental since ref.
func GetIncrementalSinceRef(env *contextdata.Envelope) (string, bool) {
	v, ok := env.GetWorkingValue(KeyIncrementalSinceRef)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// SetIncrementalSinceRef stores the incremental since ref.
func SetIncrementalSinceRef(env *contextdata.Envelope, ref string) {
	env.SetWorkingValue(KeyIncrementalSinceRef, ref, contextdata.MemoryClassTask)
}

// GetIngestPolicy retrieves the ingest policy.
func GetIngestPolicy(env *contextdata.Envelope) (string, bool) {
	v, ok := env.GetWorkingValue(KeyIngestPolicy)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// SetIngestPolicy stores the ingest policy.
func SetIngestPolicy(env *contextdata.Envelope, policy string) {
	env.SetWorkingValue(KeyIngestPolicy, policy, contextdata.MemoryClassTask)
}

// --- Intent Signals ---

// GetIntentSignals retrieves family scores from classification.
func GetIntentSignals(env *contextdata.Envelope) (map[string]float64, bool) {
	v, ok := env.GetWorkingValue(KeyIntentSignals)
	if !ok {
		return nil, false
	}
	s, ok := v.(map[string]float64)
	return s, ok
}

// SetIntentSignals stores family scores from classification.
func SetIntentSignals(env *contextdata.Envelope, scores map[string]float64) {
	env.SetWorkingValue(KeyIntentSignals, scores, contextdata.MemoryClassTask)
}

// SetThoughtRecipeID stores the thoughtrecipe ID.
func SetThoughtRecipeID(env *contextdata.Envelope, id string) {
	env.SetWorkingValue(KeyThoughtRecipeID, id, contextdata.MemoryClassTask)
}

// --- Thought ThoughtRecipe ---

// GetThoughtRecipeID retrieves the thoughtrecipe ID.
func GetThoughtRecipeID(env *contextdata.Envelope) (string, bool) {
	v, ok := env.GetWorkingValue(KeyThoughtRecipeID)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// GetThoughtRecipeVersion retrieves the thoughtrecipe version.
func GetThoughtRecipeVersion(env *contextdata.Envelope) (string, bool) {
	v, ok := env.GetWorkingValue(KeyThoughtRecipeVersion)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// SetThoughtRecipeVersion stores the thoughtrecipe version.
func SetThoughtRecipeVersion(env *contextdata.Envelope, version string) {
	env.SetWorkingValue(KeyThoughtRecipeVersion, version, contextdata.MemoryClassTask)
}

// --- Policy ---

// GetHITLTriggered retrieves whether HITL was triggered.
func GetHITLTriggered(env *contextdata.Envelope) (bool, bool) {
	v, ok := env.GetWorkingValue(KeyHITLTriggered)
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// GetHITLResponse retrieves the HITL response.
func GetHITLResponse(env *contextdata.Envelope) (*interaction.HITLResponse, bool) {
	v, ok := env.GetWorkingValue(KeyHITLResponse)
	if !ok {
		return nil, false
	}
	resp, ok := v.(*interaction.HITLResponse)
	return resp, ok
}

// SetHITLResponse stores the HITL response.
func SetHITLResponse(env *contextdata.Envelope, resp *interaction.HITLResponse) {
	env.SetWorkingValue(KeyHITLResponse, resp, contextdata.MemoryClassTask)
}

// SetHITLTriggered stores whether HITL was triggered.
func SetHITLTriggered(env *contextdata.Envelope, triggered bool) {
	env.SetWorkingValue(KeyHITLTriggered, triggered, contextdata.MemoryClassTask)
}

// GetPolicyDecision retrieves the policy decision.
func GetPolicyDecision(env *contextdata.Envelope) (*policy.PolicyDecision, bool) {
	v, ok := env.GetWorkingValue(KeyPolicyDecision)
	if !ok {
		return nil, false
	}
	pd, ok := v.(*policy.PolicyDecision)
	return pd, ok
}

// SetPolicyDecision stores the policy decision.
func SetPolicyDecision(env *contextdata.Envelope, pd *policy.PolicyDecision) {
	env.SetWorkingValue(KeyPolicyDecision, pd, contextdata.MemoryClassTask)
}

// --- Execution ---

// GetDryRunMode retrieves dry run mode.
func GetDryRunMode(env *contextdata.Envelope) (bool, bool) {
	v, ok := env.GetWorkingValue(KeyDryRunMode)
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// SetDryRunMode stores dry run mode.
func SetDryRunMode(env *contextdata.Envelope, dryRun bool) {
	env.SetWorkingValue(KeyDryRunMode, dryRun, contextdata.MemoryClassTask)
}

// GetOutcomeCategory retrieves the outcome category.
func GetOutcomeCategory(env *contextdata.Envelope) (string, bool) {
	v, ok := env.GetWorkingValue(KeyOutcomeCategory)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// SetOutcomeCategory stores the outcome category.
func SetOutcomeCategory(env *contextdata.Envelope, category string) {
	env.SetWorkingValue(KeyOutcomeCategory, category, contextdata.MemoryClassTask)
}

// GetOutcomeArtifacts retrieves outcome artifacts.
func GetOutcomeArtifacts(env *contextdata.Envelope) ([]string, bool) {
	v, ok := env.GetWorkingValue(KeyOutcomeArtifacts)
	if !ok {
		return nil, false
	}
	s, ok := v.([]string)
	return s, ok
}

// SetOutcomeArtifacts stores outcome artifacts.
func SetOutcomeArtifacts(env *contextdata.Envelope, artifacts []string) {
	env.SetWorkingValue(KeyOutcomeArtifacts, artifacts, contextdata.MemoryClassTask)
}

// GetOutcomeTelemetry retrieves outcome telemetry.
func GetOutcomeTelemetry(env *contextdata.Envelope) (map[string]any, bool) {
	v, ok := env.GetWorkingValue(KeyOutcomeTelemetry)
	if !ok {
		return nil, false
	}
	m, ok := v.(map[string]any)
	return m, ok
}

// SetOutcomeTelemetry stores outcome telemetry.
func SetOutcomeTelemetry(env *contextdata.Envelope, telemetry map[string]any) {
	env.SetWorkingValue(KeyOutcomeTelemetry, telemetry, contextdata.MemoryClassTask)
}

// --- Resume (Session Restoration) ---

// GetResumeClassification retrieves the resume classification.
func GetResumeClassification(env *contextdata.Envelope) (*intake.IntentClassification, bool) {
	v, ok := env.GetWorkingValue(KeyResumeClassification)
	if !ok {
		return nil, false
	}
	ic, ok := v.(*intake.IntentClassification)
	return ic, ok
}

// SetResumeClassification stores the resume classification.
func SetResumeClassification(env *contextdata.Envelope, ic *intake.IntentClassification) {
	env.SetWorkingValue(KeyResumeClassification, ic, contextdata.MemoryClassTask)
}

// GetResumeRoute retrieves the resume route.
func GetResumeRoute(env *contextdata.Envelope) (*orchestrate.RouteSelection, bool) {
	v, ok := env.GetWorkingValue(KeyResumeRoute)
	if !ok {
		return nil, false
	}
	rs, ok := v.(*orchestrate.RouteSelection)
	return rs, ok
}

// SetResumeRoute stores the resume route.
func SetResumeRoute(env *contextdata.Envelope, rs *orchestrate.RouteSelection) {
	env.SetWorkingValue(KeyResumeRoute, rs, contextdata.MemoryClassTask)
}

// --- Stream ---

// GetStreamTokenUsage retrieves stream token usage.
func GetStreamTokenUsage(env *contextdata.Envelope) (int, bool) {
	v, ok := env.GetWorkingValue(KeyStreamTokenUsage)
	if !ok {
		return 0, false
	}
	n, ok := v.(int)
	return n, ok
}

// SetStreamTokenUsage stores stream token usage.
func SetStreamTokenUsage(env *contextdata.Envelope, usage int) {
	env.SetWorkingValue(KeyStreamTokenUsage, usage, contextdata.MemoryClassTask)
}

// GetDispatchRouteKind retrieves the dispatch route kind.
func GetDispatchRouteKind(env *contextdata.Envelope) (string, bool) {
	v, ok := env.GetWorkingValue(KeyDispatchRouteKind)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// SetDispatchRouteKind stores the dispatch route kind.
func SetDispatchRouteKind(env *contextdata.Envelope, kind string) {
	env.SetWorkingValue(KeyDispatchRouteKind, kind, contextdata.MemoryClassTask)
}

// --- Frame History ---

// GetFrameHistory retrieves the frame history.
func GetFrameHistory(env *contextdata.Envelope) ([]string, bool) {
	v, ok := env.GetWorkingValue(KeyFrameHistory)
	if !ok {
		return nil, false
	}
	s, ok := v.([]string)
	return s, ok
}

// SetFrameHistory stores the frame history.
func SetFrameHistory(env *contextdata.Envelope, frames []string) {
	env.SetWorkingValue(KeyFrameHistory, frames, contextdata.MemoryClassTask)
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
	v, ok := env.GetWorkingValue(KeyJobRecords)
	if !ok {
		return nil, false
	}
	records, ok := v.([]JobRecord)
	return records, ok
}

// SetJobRecords stores the job history.
func SetJobRecords(env *contextdata.Envelope, records []JobRecord) {
	env.SetWorkingValue(KeyJobRecords, records, contextdata.MemoryClassTask)
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
	v, ok := env.GetWorkingValue(KeyNegativeConstraints)
	if !ok {
		return nil, false
	}
	constraints, ok := v.([]string)
	return constraints, ok
}

// SetNegativeConstraints stores the negative constraint seeds.
func SetNegativeConstraints(env *contextdata.Envelope, constraints []string) {
	env.SetWorkingValue(KeyNegativeConstraints, constraints, contextdata.MemoryClassTask)
}

// --- Family Selection (Resume) ---

// GetFamilySelection retrieves the selected family.
func GetFamilySelection(env *contextdata.Envelope) (string, bool) {
	v, ok := env.GetWorkingValue(KeyFamilySelection)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// SetFamilySelection stores the selected family.
func SetFamilySelection(env *contextdata.Envelope, family string) {
	env.SetWorkingValue(KeyFamilySelection, family, contextdata.MemoryClassTask)
}
