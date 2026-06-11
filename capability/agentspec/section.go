package agentspec

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// DecodeSection decodes an agent spec section (yaml.Node) into a typed
// AgentRuntimeSpec. Returns nil (no error) when the node is absent.
func DecodeSection(node yaml.Node) (*AgentRuntimeSpec, error) {
	if node.Kind == 0 {
		return nil, nil
	}
	var spec AgentRuntimeSpec
	if err := node.Decode(&spec); err != nil {
		return nil, fmt.Errorf("decode agent spec section: %w", err)
	}
	return &spec, nil
}
