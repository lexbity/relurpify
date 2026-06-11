package context

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// DecodeContextPolicy decodes a raw context policy section (yaml.Node) into a
// typed ContextPolicyBundle. Returns nil (no error) when the node is absent.
func DecodeContextPolicy(node yaml.Node) (*ContextPolicyBundle, error) {
	if node.Kind == 0 {
		return nil, nil
	}
	var bundle ContextPolicyBundle
	if err := node.Decode(&bundle); err != nil {
		return nil, fmt.Errorf("decode context policy: %w", err)
	}
	return &bundle, nil
}
