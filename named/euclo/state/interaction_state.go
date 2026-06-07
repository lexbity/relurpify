package state

import (
	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// GetInteractionFrameSeq returns the current interaction frame sequence counter.
func GetInteractionFrameSeq(env *contextdata.Envelope) (int, bool) {
	return contextdata.GetTyped[int](env, KeyInteractionFrameSeq)
}

// SetInteractionFrameSeq stores the interaction frame sequence counter.
func SetInteractionFrameSeq(env *contextdata.Envelope, seq int) {
	contextdata.SetTyped(env, KeyInteractionFrameSeq, seq)
}

// GetInteractionFrameRequested reports whether an interaction frame has been requested
// in the current execution cycle.
func GetInteractionFrameRequested(env *contextdata.Envelope) bool {
	v, _ := contextdata.GetTyped[bool](env, KeyInteractionFrameRequested)
	return v
}

// SetInteractionFrameRequested marks whether an interaction frame has been requested.
func SetInteractionFrameRequested(env *contextdata.Envelope, requested bool) {
	contextdata.SetTyped(env, KeyInteractionFrameRequested, requested)
}

// GetInteractionResumeNodeID returns the graph node ID at which execution should resume
// after an interaction pause.
func GetInteractionResumeNodeID(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyInteractionResumeNodeID)
	return v
}

// SetInteractionResumeNodeID stores the resume node ID.
func SetInteractionResumeNodeID(env *contextdata.Envelope, nodeID string) {
	contextdata.SetTyped(env, KeyInteractionResumeNodeID, nodeID)
}

// GetInteractionPause reports whether the execution graph is paused awaiting interaction.
func GetInteractionPause(env *contextdata.Envelope) bool {
	v, _ := contextdata.GetTyped[bool](env, KeyInteractionPause)
	return v
}

// SetInteractionPause marks the execution as paused (true) or resumed (false).
func SetInteractionPause(env *contextdata.Envelope, paused bool) {
	contextdata.SetTyped(env, KeyInteractionPause, paused)
}

// GetAskQuestion returns the pending question text for a HITL ask prompt.
func GetAskQuestion(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyAskQuestion)
	return v
}

// SetAskQuestion stores the question text for a HITL ask prompt.
func SetAskQuestion(env *contextdata.Envelope, question string) {
	contextdata.SetTyped(env, KeyAskQuestion, question)
}

// GetAskChoices returns the predefined choices for a HITL ask prompt, if any.
func GetAskChoices(env *contextdata.Envelope) ([]string, bool) {
	return contextdata.GetTyped[[]string](env, KeyAskChoices)
}

// SetAskChoices stores the predefined choices for a HITL ask prompt.
func SetAskChoices(env *contextdata.Envelope, choices []string) {
	contextdata.SetTyped(env, KeyAskChoices, choices)
}

// GetAskChoiceSource returns the source identifier that generated the ask choices.
func GetAskChoiceSource(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, KeyAskChoiceSource)
	return v
}

// SetAskChoiceSource stores the source identifier for ask choices.
func SetAskChoiceSource(env *contextdata.Envelope, source string) {
	contextdata.SetTyped(env, KeyAskChoiceSource, source)
}
