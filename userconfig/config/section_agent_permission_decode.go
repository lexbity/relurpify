package config

import (
	"fmt"

	configpermissions "codeburg.org/lexbit/relurpify/userconfig/permissions"
	"gopkg.in/yaml.v3"
)

// DecodeAgentSection decodes an agent YAML section node into a typed AgentSpec.
func DecodeAgentSection(node yaml.Node) (*AgentSpec, error) {
	if node.Kind == 0 {
		return nil, fmt.Errorf("agent section node is absent")
	}
	var spec AgentSpec
	if err := node.Decode(&spec); err != nil {
		return nil, fmt.Errorf("decode agent section: %w", err)
	}
	return &spec, nil
}

// DecodePermissionsSection decodes a permissions YAML section node into a typed PermissionSet.
func DecodePermissionsSection(node yaml.Node) (*configpermissions.PermissionSet, error) {
	if node.Kind == 0 {
		return nil, fmt.Errorf("permissions section node is absent")
	}
	var ps configpermissions.PermissionSet
	if err := node.Decode(&ps); err != nil {
		return nil, fmt.Errorf("decode permissions section: %w", err)
	}
	return &ps, nil
}
