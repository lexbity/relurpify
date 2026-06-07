package state

import (
	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// GetIngestionUserFilesCount returns the number of user files ingested.
func GetIngestionUserFilesCount(env *contextdata.Envelope) int {
	v, _ := contextdata.GetTyped[int](env, KeyIngestionUserFilesCount)
	return v
}

// SetIngestionUserFilesCount stores the user files ingested count.
func SetIngestionUserFilesCount(env *contextdata.Envelope, count int) {
	contextdata.SetTyped(env, KeyIngestionUserFilesCount, count)
}

// GetIngestionSessionPinsCount returns the number of session pins ingested.
func GetIngestionSessionPinsCount(env *contextdata.Envelope) int {
	v, _ := contextdata.GetTyped[int](env, KeyIngestionSessionPinsCount)
	return v
}

// SetIngestionSessionPinsCount stores the session pins ingested count.
func SetIngestionSessionPinsCount(env *contextdata.Envelope, count int) {
	contextdata.SetTyped(env, KeyIngestionSessionPinsCount, count)
}

// SetIngestedFile stores an ingested file's content under its path key.
// The key is KeyIngestedFilePrefix + filePath.
func SetIngestedFile(env *contextdata.Envelope, filePath, content string) {
	contextdata.SetTyped(env, KeyIngestedFilePrefix+filePath, content)
}

// SetIngestedPin stores an ingested session pin's content under its path key.
// The key is KeyIngestedPinPrefix + filePath.
func SetIngestedPin(env *contextdata.Envelope, filePath, content string) {
	contextdata.SetTyped(env, KeyIngestedPinPrefix+filePath, content)
}
