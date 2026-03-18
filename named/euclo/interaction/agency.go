package interaction

import "strings"

// AgencyTrigger maps user phrases to capability invocations or phase jumps.
type AgencyTrigger struct {
	Phrases      []string // recognized phrases (matched case-insensitively)
	CapabilityID string   // relurpic capability to invoke (optional)
	PhaseJump    string   // phase to jump to (optional)
	RequiresMode string   // only active in this mode (empty = any mode)
	Description  string   // for help surface
}

// AgencyResolver checks user freetext responses against registered triggers
// for the current mode and resolves matches.
type AgencyResolver struct {
	triggers []agencyEntry
}

type agencyEntry struct {
	mode    string
	trigger AgencyTrigger
}

// NewAgencyResolver creates a new empty resolver.
func NewAgencyResolver() *AgencyResolver {
	return &AgencyResolver{}
}

// RegisterTrigger adds a trigger for a specific mode.
func (r *AgencyResolver) RegisterTrigger(mode string, trigger AgencyTrigger) {
	r.triggers = append(r.triggers, agencyEntry{mode: mode, trigger: trigger})
}

// Resolve checks user text against registered triggers for the given mode.
// Returns the matched trigger and true, or nil and false.
func (r *AgencyResolver) Resolve(mode, userText string) (*AgencyTrigger, bool) {
	normalized := strings.ToLower(strings.TrimSpace(userText))
	if normalized == "" {
		return nil, false
	}

	for i := range r.triggers {
		entry := &r.triggers[i]
		if entry.mode != "" && entry.mode != mode {
			continue
		}
		if entry.trigger.RequiresMode != "" && entry.trigger.RequiresMode != mode {
			continue
		}
		for _, phrase := range entry.trigger.Phrases {
			if strings.ToLower(phrase) == normalized {
				return &entry.trigger, true
			}
		}
	}

	// Fuzzy: check if user text contains any trigger phrase.
	for i := range r.triggers {
		entry := &r.triggers[i]
		if entry.mode != "" && entry.mode != mode {
			continue
		}
		if entry.trigger.RequiresMode != "" && entry.trigger.RequiresMode != mode {
			continue
		}
		for _, phrase := range entry.trigger.Phrases {
			if strings.Contains(normalized, strings.ToLower(phrase)) {
				return &entry.trigger, true
			}
		}
	}

	return nil, false
}

// TriggersForMode returns all triggers registered for the given mode,
// suitable for building the help surface.
func (r *AgencyResolver) TriggersForMode(mode string) []AgencyTrigger {
	var out []AgencyTrigger
	for _, entry := range r.triggers {
		if entry.mode == mode || entry.mode == "" {
			out = append(out, entry.trigger)
		}
	}
	return out
}
