package openaicompat

import (
	"strings"
	"time"
)

// OpenAICompatConfig configures an OpenAI-compatible backend client.
type OpenAICompatConfig struct {
	Endpoint          string        `yaml:"endpoint" json:"endpoint"`
	APIKey            string        `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Timeout           time.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	NativeToolCalling bool          `yaml:"native_tool_calling,omitempty" json:"native_tool_calling,omitempty"`
}

func (c OpenAICompatConfig) normalizedEndpoint() string {
	endpoint := strings.TrimSpace(c.Endpoint)
	return strings.TrimRight(endpoint, "/")
}
