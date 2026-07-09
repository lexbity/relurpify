package main

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/platform/tools/native"
)

// init registers the built-in native (go_native) tool implementations so the
// euclo agent's capability registry is populated when run via dev-agent-cli.
// The production runtime (relurpish) achieves the same effect through blank
// imports in app/relurpish/runtime/tool_activation.go; dev-agent-cli must do
// the equivalent or every go_native tool manifest is skipped in strict mode,
// leaving the dispatch node with no eligible route candidates.
func init() {
	for name, ctor := range native.AllConstructors() {
		ports.RegisterNativeNoPanic(name, ctor)
	}
}
