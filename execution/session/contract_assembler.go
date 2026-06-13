package session

import (
	"errors"
	"fmt"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	execctx "codeburg.org/lexbit/relurpify/execution/context"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	"gopkg.in/yaml.v3"
)

// AssembleContract builds an EffectiveAgentContract from a Document by
// decoding each known spec section through its domain's DecodeSection.
// Unknown sections are silently ignored.
func AssembleContract(doc *config.Document) (*config.EffectiveAgentContract, error) {
	if doc == nil {
		return nil, fmt.Errorf("document required")
	}

	agentID := doc.Metadata.Name

	// Decode permissions section
	perms, err := decodePermissionsSection(doc)
	if err != nil {
		return nil, fmt.Errorf("decode permissions: %w", err)
	}

	// Decode resource section
	resources, err := decodeResourceSection(doc)
	if err != nil {
		return nil, fmt.Errorf("decode resources: %w", err)
	}

	// Decode security section
	security, err := decodeSecuritySection(doc)
	if err != nil {
		return nil, fmt.Errorf("decode security: %w", err)
	}

	// Decode agent spec section
	agentSpec, err := decodeAgentSection(doc)
	if err != nil {
		return nil, fmt.Errorf("decode agent: %w", err)
	}
	if agentSpec == nil {
		return nil, fmt.Errorf("agent section required")
	}

	// Decode context policy section (best-effort, optional)
	contextPolicy, _ := decodeContextPolicySection(doc)
	_ = contextPolicy

	sources := config.SourceSummary{
		ManifestName:    doc.Metadata.Name,
		ManifestVersion: doc.Metadata.Version,
	}

	return config.BuildEffectiveAgentContract(agentID, agentSpec, perms, resources, security, sources), nil
}

func decodePermissionsSection(doc *config.Document) (permissions.PermissionSet, error) {
	node, ok := doc.Section("permissions")
	if !ok {
		return permissions.PermissionSet{}, nil
	}
	ps, err := permissions.DecodeSection(node)
	if err != nil || ps == nil {
		return permissions.PermissionSet{}, err
	}
	return *ps, nil
}

func decodeAgentSection(doc *config.Document) (*agentspec.AgentRuntimeSpec, error) {
	node, ok := doc.Section("agent")
	if !ok {
		return nil, errors.New("agent section not found")
	}
	return agentspec.DecodeSection(node)
}

func decodeResourceSection(doc *config.Document) (config.ResourceSpec, error) {
	node, ok := doc.Section("resources")
	if !ok {
		return config.ResourceSpec{}, nil
	}
	rs, err := config.DecodeResourceSection(node)
	if err != nil || rs == nil {
		return config.ResourceSpec{}, err
	}
	return *rs, nil
}

func decodeSecuritySection(doc *config.Document) (config.SecuritySpec, error) {
	node, ok := doc.Section("security")
	if !ok {
		return config.SecuritySpec{}, nil
	}
	ss, err := config.DecodeSecuritySection(node)
	if err != nil || ss == nil {
		return config.SecuritySpec{}, err
	}
	return *ss, nil
}

func decodeContextPolicySection(doc *config.Document) (*execctx.ContextPolicyBundle, error) {
	node, ok := doc.Section("context")
	if !ok {
		return nil, errors.New("context section not found")
	}
	return execctx.DecodeContextPolicy(node)
}

// DecodeYAMLNode is a helper that decodes a yaml.Node into a typed value via
// the standard yaml.Node.Decode method. It panics if the node is zero-valued.
func DecodeYAMLNode[T any](node yaml.Node) (*T, error) {
	if node.Kind == 0 {
		return nil, errors.New("yaml node is zero-valued")
	}
	var out T
	if err := node.Decode(&out); err != nil {
		return nil, fmt.Errorf("decode yaml node: %w", err)
	}
	return &out, nil
}
