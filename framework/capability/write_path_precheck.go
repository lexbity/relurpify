package capability

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/search"
)

// InvocationPrecheck is checked after policy evaluation and before invocation.
// Returning a non-nil error blocks the call with that error message.
type InvocationPrecheck interface {
	Check(descriptor core.CapabilityDescriptor, args map[string]any) error
}

// PostInvocationHook receives the completed invocation result.
type PostInvocationHook interface {
	Record(descriptor core.CapabilityDescriptor, result *core.ToolResult) error
}

// WritePathPrecheck blocks filesystem-mutating capabilities from writing to
// paths outside an allowed glob list. Nil globs disable the check.
type WritePathPrecheck struct {
	Globs []string
}

func (p WritePathPrecheck) Check(desc core.CapabilityDescriptor, args map[string]any) error {
	if len(p.Globs) == 0 || !hasWriteEffect(desc) {
		return nil
	}
	path, ok := extractPathArg(args)
	if !ok {
		return fmt.Errorf("write restricted to documentation paths; cannot determine target path")
	}
	for _, glob := range p.Globs {
		if search.MatchGlob(glob, path) {
			return nil
		}
	}
	return fmt.Errorf("write to %q blocked: mode restricts writes to documentation paths (%s)", path, strings.Join(p.Globs, ", "))
}

func hasWriteEffect(desc core.CapabilityDescriptor) bool {
	for _, effect := range desc.EffectClasses {
		if effect == core.EffectClassFilesystemMutation {
			return true
		}
	}
	return false
}

func extractPathArg(args map[string]any) (string, bool) {
	for _, key := range []string{"path", "file_path", "target", "filename", "dest"} {
		value, ok := args[key]
		if !ok {
			continue
		}
		path, ok := value.(string)
		if ok && strings.TrimSpace(path) != "" {
			return path, true
		}
	}
	return "", false
}
