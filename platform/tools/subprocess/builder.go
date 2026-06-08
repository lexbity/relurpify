package subprocess

import "codeburg.org/lexbit/relurpify/capability/ports"

type builder struct{}

func (builder) BuildTool(manifest ports.ToolManifest, runner ports.CommandRunner) (ports.Tool, error) {
	return NewTool(manifest, runner), nil
}

func BackendBuilder() ports.ToolBackendBuilder { return builder{} }
