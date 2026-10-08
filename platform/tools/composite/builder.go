package composite

import (
	"errors"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// builder is the composite ToolBackendBuilder. It carries the tool resolver
// used at execution time to locate each step's implementation.
type builder struct {
	resolver ToolResolver
}

func (b builder) BuildTool(manifest ports.ToolManifest, _ ports.CommandRunner) (ports.Tool, error) {
	if b.resolver == nil {
		return nil, errors.New("composite backend requires a tool resolver")
	}
	return New(manifest, b.resolver), nil
}

// BackendBuilder returns the composite ToolBackendBuilder. The resolver resolves
// sub-tool names to their runtime implementations at execution time; the
// composition root wires it to the capability registry so composite steps
// actually execute instead of failing with "tool not found".
func BackendBuilder(resolver ToolResolver) ports.ToolBackendBuilder {
	return builder{resolver: resolver}
}
