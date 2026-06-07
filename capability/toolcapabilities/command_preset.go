package toolcapabilities

import (
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// Tag constants classify tools for policy enforcement.
const (
	TagReadOnly    = ports.TagReadOnly
	TagExecute     = ports.TagExecute
	TagDestructive = ports.TagDestructive
	TagNetwork     = ports.TagNetwork
)

// CommandPreset describes a reusable command wrapper for shell tools.
type CommandPreset struct {
	Name        string
	Command     string
	DefaultArgs []string
	Description string
	Category    string
	Tags        []string
	Timeout     time.Duration
	AllowStdin  bool
	AllowFlags  bool
	WorkdirMode string
}

// NewCommandPreset normalizes a CommandPreset with sensible defaults.
func NewCommandPreset(p CommandPreset) CommandPreset {
	if p.Category == "" {
		p.Category = "cli"
	}
	if p.Timeout <= 0 {
		p.Timeout = 60 * time.Second
	}
	if p.WorkdirMode == "" {
		p.WorkdirMode = "workspace"
	}
	return p
}
