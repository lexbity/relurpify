package capresult

// RevocationSnapshot captures the current set of revoked capabilities, providers, and sessions.
type RevocationSnapshot struct {
	Capabilities map[string]string `json:"capabilities,omitempty"`
	Providers    map[string]string `json:"providers,omitempty"`
	Sessions     map[string]string `json:"sessions,omitempty"`
}
