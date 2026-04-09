package lmstudio

import "time"

// Config captures construction settings for the LM Studio transport backend.
type Config struct {
	Endpoint          string
	Model             string
	APIKey            string
	Timeout           time.Duration
	NativeToolCalling bool
	Debug             bool
}
