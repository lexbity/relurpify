package native

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/platform/tools/native/fs"
	"codeburg.org/lexbit/relurpify/platform/tools/native/git"
	"codeburg.org/lexbit/relurpify/platform/tools/native/langgo"
	"codeburg.org/lexbit/relurpify/platform/tools/native/langjs"
	"codeburg.org/lexbit/relurpify/platform/tools/native/langpython"
	"codeburg.org/lexbit/relurpify/platform/tools/native/langrust"
	"codeburg.org/lexbit/relurpify/platform/tools/native/lsp"
	"codeburg.org/lexbit/relurpify/platform/tools/native/search"
	"codeburg.org/lexbit/relurpify/platform/tools/native/sqlite"
)

// AllConstructors returns a map of all built-in native tool constructors keyed
// by normalized name. This replaces the implicit init()-based registration
// pattern with explicit construction. The returned map is a snapshot; callers
// can select subsets or pass the entire map to ports.RegisterNativeNoPanic.
func AllConstructors() map[string]ports.NativeToolConstructor {
	out := make(map[string]ports.NativeToolConstructor)
	collect := func(m map[string]ports.NativeToolConstructor) {
		for k, v := range m {
			out[k] = v
		}
	}
	collect(fs.Constructors())
	collect(git.Constructors())
	collect(langgo.Constructors())
	collect(langjs.Constructors())
	collect(langpython.Constructors())
	collect(langrust.Constructors())
	collect(lsp.Constructors())
	collect(search.Constructors())
	collect(sqlite.Constructors())
	return out
}
