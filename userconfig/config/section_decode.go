package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// DecodeResourceSection decodes a resources section node into a ResourceSpec.
func DecodeResourceSection(node yaml.Node) (*ResourceSpec, error) {
	if node.Kind == 0 {
		return nil, fmt.Errorf("resources section node is absent")
	}
	var spec ResourceSpec
	if err := node.Decode(&spec); err != nil {
		return nil, fmt.Errorf("decode resources section: %w", err)
	}
	return &spec, nil
}

// DecodeSecuritySection decodes a security section node into a SecuritySpec.
func DecodeSecuritySection(node yaml.Node) (*SecuritySpec, error) {
	if node.Kind == 0 {
		return nil, fmt.Errorf("security section node is absent")
	}
	var spec SecuritySpec
	if err := node.Decode(&spec); err != nil {
		return nil, fmt.Errorf("decode security section: %w", err)
	}
	return &spec, nil
}
