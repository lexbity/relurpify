package templates

import (
	"fmt"
	"io/fs"
	"strings"

	"codeburg.org/lexbit/relurpify/userconfig/templates/embedfs"
)

// Resolver discovers installed shared templates and falls back to repo-local
// development templates while the install model is being phased in.
type Resolver struct {
	sharedRoot string
}

// NewResolver returns a resolver with an explicit shared template root.
func NewResolver(sharedRoot string) Resolver {
	return Resolver{sharedRoot: sharedRoot}
}

// ResolveTestsuiteTemplateProfile resolves the relurpify_cfg root for a named
// testsuite template profile from the embedded FS.
func (r Resolver) ResolveTestsuiteTemplateProfile(name string) (fs.FS, error) {
	if strings.TrimSpace(name) == "" {
		name = "default"
	}
	if name != "default" {
		return nil, fmt.Errorf("unknown testsuite profile %q", name)
	}
	return fs.Sub(embedfs.DefaultFS(), "workspace")
}
