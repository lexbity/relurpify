package agentenv

import (
	"log"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
)

// synthesizeManifestSnapshot creates a minimal in-memory manifest snapshot
// from an agent spec. This allows BootstrapAgentRuntime to work without a
// file-backed manifest, enabling embedded agents (rex/nexus) to go through
// the same security foundation as full entry points.
func synthesizeManifestSnapshot(agentName string, spec *agentspec.AgentRuntimeSpec) *cfgload.AgentManifestSnapshot {
	name := agentName
	if name == "" {
		name = "agent"
	}
	log.Printf("synthesizing minimal manifest snapshot for agent %q (spec-only bootstrap)", name)

	return &cfgload.AgentManifestSnapshot{
		Manifest: &cfgload.AgentManifest{
			APIVersion: "relurpify/v1alpha1",
			Kind:       "AgentManifest",
			Metadata: cfgload.ManifestMetadata{
				Name: name,
			},
			Spec: cfgload.ManifestSpec{
				Agent: spec,
			},
		},
	}
}
