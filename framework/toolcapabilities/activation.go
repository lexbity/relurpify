// Package toolcapabilities activation — blank imports to fire init()
// registration of native tool constructors into contracts.NativeRegistry.

package toolcapabilities

import (
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/fs"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/git"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/langgo"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/langjs"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/langpython"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/langrust"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/lsp"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/search"
	_ "codeburg.org/lexbit/relurpify/platform/tools/native/sqlite"
)
