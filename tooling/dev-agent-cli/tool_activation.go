package main

import (
	"fmt"
	"os"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/platform/tools/native"
)

// init registers the built-in native (go_native) tool implementations so the
// euclo agent's capability registry is populated when run via dev-agent-cli.
// The production runtime (relurpish) achieves the same effect through blank
// imports in app/relurpish/runtime/tool_activation.go; dev-agent-cli does the
// equivalent or every go_native tool manifest is skipped in strict mode,
// leaving the dispatch node with no eligible route candidates.
func init() {
	for name, ctor := range native.AllConstructors() {
		// The root package's own imports already ran the subpackage init()
		// registrations; a duplicate here means a key collision between
		// native tool families, which must be visible, not swallowed.
		if _, registered := ports.LookupNative(name); registered {
			continue
		}
		if err := ports.RegisterNativeNoPanic(name, ctor); err != nil {
			fmt.Fprintf(os.Stderr, "dev-agent-cli: register native tool %q: %v\n", name, err)
		}
	}
}
