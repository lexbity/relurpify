package session

import (
	"log"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	"gopkg.in/yaml.v3"
)

const defaultSynthesizedAgentName = "agent"

// synthesizeDocumentSnapshot creates a minimal in-memory document snapshot
// from an agent spec so embedded agents can resolve an effective contract
// through the document path.
func synthesizeDocumentSnapshot(agentName string, spec *agentspec.AgentRuntimeSpec) *config.DocumentSnapshot {
	name := agentName
	if name == "" {
		name = defaultSynthesizedAgentName
	}
	log.Printf("synthesizing minimal document snapshot for agent %q (spec-only bootstrap)", name)

	var agentNode yaml.Node
	if err := agentNode.Encode(spec); err != nil {
		return &config.DocumentSnapshot{
			Document: &config.Document{
				APIVersion: "relurpify.io/v1",
				Kind:       "AgentManifest",
				Metadata:   config.DocumentMetadata{Name: name},
				Spec: map[string]yaml.Node{
					"agent": {},
				},
			},
		}
	}

	return &config.DocumentSnapshot{
		Document: &config.Document{
			APIVersion: "relurpify.io/v1",
			Kind:       "AgentManifest",
			Metadata:   config.DocumentMetadata{Name: name},
			Spec: map[string]yaml.Node{
				"agent": agentNode,
			},
		},
	}
}
