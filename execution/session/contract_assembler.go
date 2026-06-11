package session

import (
	"errors"
	"fmt"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/execution/context"
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
	perms, _ := decodePermissionsSection(doc)

	// Decode agent spec section
	agentSpec, _ := decodeAgentSection(doc)

	// Decode context policy section (best-effort, optional)
	contextPolicy, _ := decodeContextPolicySection(doc)
	_ = contextPolicy

	sources := config.SourceSummary{
		ManifestName:    doc.Metadata.Name,
		ManifestVersion: doc.Metadata.Version,
	}

	return config.BuildEffectiveAgentContract(agentID, agentSpec, perms, config.ResourceSpec{}, sources), nil
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

func decodeContextPolicySection(doc *config.Document) (*execctx.ContextPolicyBundle, error) {
	node, ok := doc.Section("context")
	if !ok {
		return nil, errors.New("context section not found")
	}
	return context.DecodeContextPolicy(node)
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
