package agentenv

import (
	"log"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// synthesizeManifestSnapshot creates a minimal in-memory manifest snapshot
// from an agent spec. This allows BootstrapAgentRuntime to work without a
// file-backed manifest, enabling embedded agents (rex/nexus) to go through
// the same security foundation as full entry points.
func synthesizeManifestSnapshot(agentName string, spec *agentspec.AgentRuntimeSpec) *config.AgentManifestSnapshot {
	name := agentName
	if name == "" {
		name = "agent"
	}
	log.Printf("synthesizing minimal manifest snapshot for agent %q (spec-only bootstrap)", name)

	return &config.AgentManifestSnapshot{
		Manifest: &config.AgentManifest{
			APIVersion: "relurpify/v1alpha1",
			Kind:       "AgentManifest",
			Metadata: config.ManifestMetadata{
				Name: name,
			},
			Spec: config.ManifestSpec{
				Agent: spec,
			},
		},
	}
}
