package euclo

// EucloManifestExtension defines the user-configurable extension stanza
// for the agent.manifest.yaml under the "euclo:" key.
type EucloManifestExtension struct {
	// CapabilityKeywords maps capability IDs to additional keyword terms.
	// These are merged with the built-in keywords in DefaultRegistry().
	// Example:
	//   euclo:
	//     capability_keywords:
	//       "euclo:chat.ask": ["clarify", "elaborate"]
	//       "euclo:chat.implement": ["hack", "quickfix"]
	CapabilityKeywords map[string][]string `yaml:"capability_keywords,omitempty" json:"capability_keywords,omitempty"`
}

// EucloExtensionKey is the top-level key in agent.manifest.yaml for Euclo-specific config.
const EucloExtensionKey = "euclo"

// capabilityKeywordsFromManifest extracts capability keywords from the manifest extension.
// Returns nil if no extension or no keywords are configured.
func capabilityKeywordsFromManifest(ext *EucloManifestExtension) map[string][]string {
	if ext == nil || len(ext.CapabilityKeywords) == 0 {
		return nil
	}
	// Return a copy to prevent mutation of the manifest config
	out := make(map[string][]string, len(ext.CapabilityKeywords))
	for k, v := range ext.CapabilityKeywords {
		out[k] = append([]string(nil), v...)
	}
	return out
}
