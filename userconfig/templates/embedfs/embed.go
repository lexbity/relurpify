// Package embedfs exposes the embedded default template bundle.
// The embed FS is the canonical source for workspace config, manifest,
// security policies, tool definitions, and prompts. An installed binary
// carries these defaults and can self-provision without a source tree.
package embedfs

import (
	"embed"
	"io/fs"
)

//go:embed all:workspace all:prompts
var defaultTemplates embed.FS

// DefaultFS returns the embedded template filesystem containing all
// canonical workspace and prompt templates. Callers (e.g. doctor.go's
// InitializeWorkspaceFromTemplates) read from this FS to materialize
// the starter relurpify_cfg/ tree.
func DefaultFS() fs.FS {
	return defaultTemplates
}
