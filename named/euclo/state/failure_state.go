package state

import (
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

// SetStepFailure records one operational step failure. The latest failure is
// stored under KeyStepFailure and appended to the run's ordered failure list
// under KeyStepFailures so the recipe executor can aggregate a structured
// outcome.
func SetStepFailure(env *contextdata.Envelope, failure *euclotypes.StepFailure) {
	if env == nil || failure == nil {
		return
	}
	contextdata.SetTyped(env, KeyStepFailure, failure)
	failures := GetStepFailures(env)
	failures = append(failures, failure)
	contextdata.SetTyped(env, KeyStepFailures, failures)
}

// GetStepFailure returns the most recent operational step failure, if any.
func GetStepFailure(env *contextdata.Envelope) (*euclotypes.StepFailure, bool) {
	return contextdata.GetTyped[*euclotypes.StepFailure](env, KeyStepFailure)
}

// GetStepFailures returns every operational step failure recorded for the run,
// in step order.
func GetStepFailures(env *contextdata.Envelope) []*euclotypes.StepFailure {
	failures, ok := contextdata.GetTyped[[]*euclotypes.StepFailure](env, KeyStepFailures)
	if !ok {
		return nil
	}
	return failures
}

// SetFallbackTaken records that an authored fallback agent took over a step.
func SetFallbackTaken(env *contextdata.Envelope, taken bool) {
	contextdata.SetTyped(env, KeyFallbackTaken, taken)
}

// GetFallbackTaken reports whether an authored fallback fired during the run.
func GetFallbackTaken(env *contextdata.Envelope) bool {
	taken, _ := contextdata.GetTyped[bool](env, KeyFallbackTaken)
	return taken
}
