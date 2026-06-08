package composite

import "codeburg.org/lexbit/relurpify/capability/ports"

type builder struct{}

func (builder) BuildTool(manifest ports.ToolManifest, runner ports.CommandRunner) (ports.Tool, error) {
	resolver := func(name string) (ports.Tool, bool) {
		return nil, false
	}
	return New(manifest, resolver), nil
}

func BackendBuilder() ports.ToolBackendBuilder { return builder{} }
