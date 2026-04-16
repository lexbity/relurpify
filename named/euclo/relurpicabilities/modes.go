package relurpicabilities

// Canonical mode constants for Euclo capability routing.
// These are the single source of truth for valid mode IDs.
const (
	ModeDebug    = "debug"
	ModeReview   = "review"
	ModePlanning = "planning"
	ModeTDD      = "tdd"
	ModeCode     = "code"
	ModeChat     = "chat"
)

// AllModes returns all registered mode IDs in priority order.
func AllModes() []string {
	return []string{ModeDebug, ModeReview, ModePlanning, ModeTDD, ModeCode, ModeChat}
}

// IsValidMode checks if a mode ID is one of the canonical modes.
func IsValidMode(mode string) bool {
	switch mode {
	case ModeDebug, ModeReview, ModePlanning, ModeTDD, ModeCode, ModeChat:
		return true
	}
	return false
}
