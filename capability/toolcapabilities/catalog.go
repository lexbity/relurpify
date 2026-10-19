package toolcapabilities

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
	nativecatalog "codeburg.org/lexbit/relurpify/platform/tools/native"
)

// CatalogForWorkspace constructs every registered go_native implementation
// as a workspace tool: the capability view that vocabulary resolution seeds
// with (loader diagnostics, doctor's canonical-recipe check, corpus gates).
// Shipped recipes may declare any shipped manifest tool, so the seeded view
// must span the whole native catalog, not just one family. Construction is
// the composition root's registration (platform/tools/native), reached here
// as a downward dependency.
func CatalogForWorkspace(workspace string) ([]ports.Tool, error) {
	constructors := nativecatalog.AllConstructors()
	out := make([]ports.Tool, 0, len(constructors))
	for _, ctor := range constructors {
		tool := ctor(workspace)
		if tool == nil {
			continue
		}
		out = append(out, tool)
	}
	return out, nil
}
