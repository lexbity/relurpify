package config

import "time"

// RuntimeMode describes how rex is hosted.
type RuntimeMode string

const (
	RuntimeModeNexusManaged RuntimeMode = "nexus-managed"
	RuntimeModeEmbedded     RuntimeMode = "embedded"
)

// Config contains rex-specific defaults layered over the shared environment.
type Config struct {
	RuntimeMode        RuntimeMode
	QueueCapacity      int
	WorkerCount        int
	RecoveryScanPeriod time.Duration
	IdlePollPeriod     time.Duration
	RequireProof       bool
}

// Default returns Nexus-managed defaults for rex.
func Default() Config {
	return Config{
		RuntimeMode:        RuntimeModeNexusManaged,
		QueueCapacity:      32,
		WorkerCount:        4,
		RecoveryScanPeriod: 30 * time.Second,
		IdlePollPeriod:     200 * time.Millisecond,
		RequireProof:       true,
	}
}
