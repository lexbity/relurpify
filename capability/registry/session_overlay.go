package registry

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/capability/handler"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

const sessionCapKeyPrefix = "_session_cap:"

// RegisterSessionCapability stores a session-scoped capability handler in the
// envelope's working memory. Session capabilities take precedence over the
// global registry during invocation within the same envelope. Returns an error
// if the envelope, id, or handler is nil/empty.
//
// TODO(fail-fast): callers that declare RequiredTools upfront (non-recipe
// capabilities) should pass a *CapabilityRegistry here and validate tool
// availability before storing — matching the behaviour of
// relurpicabilities.computeAvailability. Recipe-derived handlers skip this
// check because their tool dependencies are determined at execution time.
func RegisterSessionCapability(env ports.State, id string, handler handler.InvocableCapabilityHandler) error {
	if env == nil {
		return fmt.Errorf("envelope required for session capability registration")
	}
	if id == "" {
		return fmt.Errorf("capability id required")
	}
	if handler == nil {
		return fmt.Errorf("capability handler required")
	}
	// envelope: intentional dynamic key — the capability ID is user-extensible
	// and session-local handlers must remain addressable by that composite key.
	env.SetWorkingValue(sessionCapKeyPrefix+id, handler)
	return nil
}

// LookupSessionCapability retrieves a session-scoped capability handler from
// the envelope. Returns false if no session-local handler is registered for id.
func LookupSessionCapability(env ports.State, id string) (handler.InvocableCapabilityHandler, bool) {
	if env == nil || id == "" {
		return nil, false
	}
	val, ok := env.GetWorkingValue(sessionCapKeyPrefix + id)
	if !ok {
		return nil, false
	}
	h, ok := val.(handler.InvocableCapabilityHandler)
	return h, ok
}
