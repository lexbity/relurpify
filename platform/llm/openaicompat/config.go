package openaicompat

import (
	"strings"
	"time"
)

// OpenAICompatConfig configures an OpenAI-compatible backend client.
type OpenAICompatConfig struct {
	Endpoint          string        `yaml:"endpoint" json:"endpoint"`
	Timeout           time.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	NativeToolCalling bool          `yaml:"native_tool_calling,omitempty" json:"native_tool_calling,omitempty"`
}

func (c OpenAICompatConfig) normalizedEndpoint() string {
	endpoint := strings.TrimSpace(c.Endpoint)
	endpoint = strings.TrimRight(endpoint, "/")
	// The client appends the /v1 path segments itself, so a user-supplied
	// base that already carries /v1 (vLLM, TGI, LM Studio — the documented
	// serving shape) must not keep it: it produced /v1/v1/models probes.
	endpoint = strings.TrimSuffix(endpoint, "/v1")
	endpoint = strings.TrimRight(endpoint, "/")
	return endpoint
}
