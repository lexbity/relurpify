package state

import (
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
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

// GetClassificationMetadata retrieves classification metadata.
func GetClassificationMetadata(env *contextdata.Envelope) (map[string]any, bool) {
	v, ok := env.GetWorkingValue(KeyClassificationMetadata)
	if !ok {
		return nil, false
	}
	m, ok := v.(map[string]any)
	return m, ok
}

// SetClassificationMetadata stores classification metadata.
func SetClassificationMetadata(env *contextdata.Envelope, m map[string]any) {
	env.SetWorkingValue(KeyClassificationMetadata, m, contextdata.MemoryClassTask)
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

// GetFamilyScores retrieves family scores (alias for GetIntentSignals).
func GetFamilyScores(env *contextdata.Envelope) (map[string]float64, bool) {
	return GetIntentSignals(env)
}

// SetFamilyScores stores family scores (alias for SetIntentSignals).
func SetFamilyScores(env *contextdata.Envelope, scores map[string]float64) {
	SetIntentSignals(env, scores)
}

// SetRecipeID stores the recipe ID.
func SetRecipeID(env *contextdata.Envelope, id string) {
	env.SetWorkingValue(KeyRecipeID, id, contextdata.MemoryClassTask)
}

// --- Thought Recipe ---

// GetRecipeID retrieves the recipe ID.
func GetRecipeID(env *contextdata.Envelope) (string, bool) {
	v, ok := env.GetWorkingValue(KeyRecipeID)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// GetRecipeVersion retrieves the recipe version.
func GetRecipeVersion(env *contextdata.Envelope) (string, bool) {
	v, ok := env.GetWorkingValue(KeyRecipeVersion)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// SetRecipeVersion stores the recipe version.
func SetRecipeVersion(env *contextdata.Envelope, version string) {
	env.SetWorkingValue(KeyRecipeVersion, version, contextdata.MemoryClassTask)
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

// SetHITLTriggered stores whether HITL was triggered.
func SetHITLTriggered(env *contextdata.Envelope, triggered bool) {
	env.SetWorkingValue(KeyHITLTriggered, triggered, contextdata.MemoryClassTask)
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

// --- Capability Sequence ---

// GetCapabilitySequence retrieves the capability sequence.
func GetCapabilitySequence(env *contextdata.Envelope) ([]string, bool) {
	v, ok := env.GetWorkingValue(KeyCapabilitySequence)
	if !ok {
		return nil, false
	}
	seq, ok := v.([]string)
	return seq, ok
}

// SetCapabilitySequence stores the capability sequence.
func SetCapabilitySequence(env *contextdata.Envelope, sequence []string) {
	env.SetWorkingValue(KeyCapabilitySequence, sequence, contextdata.MemoryClassTask)
}
