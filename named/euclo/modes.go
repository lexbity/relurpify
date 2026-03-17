package euclo

import "github.com/lexcodex/relurpify/named/euclo/euclotypes"

// Re-export mode types and functions from euclotypes for backward compatibility.
type (
	ModeDescriptor = euclotypes.ModeDescriptor
	ModeRegistry   = euclotypes.ModeRegistry
	ModeResolution = euclotypes.ModeResolution
)

var (
	NewModeRegistry      = euclotypes.NewModeRegistry
	DefaultModeRegistry  = euclotypes.DefaultModeRegistry
)
