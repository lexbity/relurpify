package registry

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/handler"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

const sessionCapKeyPrefix = "_session_cap:"

// RegisterSessionCapability stores a session-scoped capability handler in the
// envelope's working memory. Session capabilities take precedence over the
// global registry during invocation within the same envelope. Returns an error
// if the envelope, id, or handler is nil/empty.
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
	desc := handler.Descriptor(context.Background(), env)
	if err := validateSessionCapabilityAvailability(id, desc); err != nil {
		return err
	}
	// envelope: intentional dynamic key — the capability ID is user-extensible
	// and session-local handlers must remain addressable by that composite key.
	env.SetWorkingValue(sessionCapKeyPrefix+id, handler)
	return nil
}

func validateSessionCapabilityAvailability(id string, desc descriptor.CapabilityDescriptor) error {
	if desc.Availability.Available {
		return nil
	}
	capID := strings.TrimSpace(id)
	if capID == "" {
		capID = strings.TrimSpace(desc.ID)
	}
	if capID == "" {
		capID = "session capability"
	}
	reason := strings.TrimSpace(desc.Availability.Reason)
	if reason == "" {
		reason = "unavailable"
	}
	return fmt.Errorf("capability %s unavailable: %s", capID, reason)
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
