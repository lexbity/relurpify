package state

import (
	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// GetBackgroundJobID returns the ID of the most recently submitted background job.
func GetBackgroundJobID(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyBackgroundJobID)
	return v
}

// SetBackgroundJobID stores the submitted job ID.
func SetBackgroundJobID(env *contextdata.Envelope, id string) {
	contextdata.SetTyped(env, KeyBackgroundJobID, id)
}

// GetBackgroundJobQueue returns the queue the job was submitted to.
func GetBackgroundJobQueue(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyBackgroundJobQueue)
	return v
}

// SetBackgroundJobQueue stores the job queue name.
func SetBackgroundJobQueue(env *contextdata.Envelope, queue string) {
	contextdata.SetTyped(env, KeyBackgroundJobQueue, queue)
}

// GetBackgroundJobKind returns the job kind (e.g. "context_stream", "ingestion").
func GetBackgroundJobKind(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyBackgroundJobKind)
	return v
}

// SetBackgroundJobKind stores the job kind.
func SetBackgroundJobKind(env *contextdata.Envelope, kind string) {
	contextdata.SetTyped(env, KeyBackgroundJobKind, kind)
}

// GetBackgroundJobSubmitted reports whether a background job has been submitted this execution.
func GetBackgroundJobSubmitted(env *contextdata.Envelope) bool {
	v, _ := contextdata.GetTyped[bool](env, KeyBackgroundJobSubmitted)
	return v
}

// SetBackgroundJobSubmitted records that a background job was submitted.
func SetBackgroundJobSubmitted(env *contextdata.Envelope, submitted bool) {
	contextdata.SetTyped(env, KeyBackgroundJobSubmitted, submitted)
}

// GetBackgroundJobState returns the job state string (mirrors jobs.JobState).
func GetBackgroundJobState(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyBackgroundJobState)
	return v
}

// SetBackgroundJobState stores the job state string.
func SetBackgroundJobState(env *contextdata.Envelope, state string) {
	contextdata.SetTyped(env, KeyBackgroundJobState, state)
}

// GetBackgroundJobCompleted reports whether the background job has finished.
func GetBackgroundJobCompleted(env *contextdata.Envelope) bool {
	v, _ := contextdata.GetTyped[bool](env, KeyBackgroundJobCompleted)
	return v
}

// SetBackgroundJobCompleted records that the background job has finished.
func SetBackgroundJobCompleted(env *contextdata.Envelope, completed bool) {
	contextdata.SetTyped(env, KeyBackgroundJobCompleted, completed)
}

// GetBackgroundJobCompletion returns the completion payload from the background job.
func GetBackgroundJobCompletion(env *contextdata.Envelope) (map[string]any, bool) {
	return contextdata.GetTyped[map[string]any](env, KeyBackgroundJobCompletion)
}

// SetBackgroundJobCompletion stores the job completion payload.
func SetBackgroundJobCompletion(env *contextdata.Envelope, data map[string]any) {
	contextdata.SetTyped(env, KeyBackgroundJobCompletion, data)
}
