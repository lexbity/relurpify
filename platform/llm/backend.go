package llm

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// Re-export contract types for local usage
type (
	LanguageModel       = contracts.LanguageModel
	BackendCapabilities = contracts.BackendCapabilities
)

// Embedder produces dense vector representations of text.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	ModelID() string
	Dims() int
}

// ManagedBackend owns the lifecycle and capability surface for a local
// inference backend.
type ManagedBackend interface {
	Model() LanguageModel
	Embedder() Embedder
	Capabilities() BackendCapabilities
	ModelContextSize(ctx context.Context) (int, error)
	Health(ctx context.Context) (*HealthReport, error)
	ListModels(ctx context.Context) ([]ModelInfo, error)
	Warm(ctx context.Context) error
	Close() error
	SetDebugLogging(enabled bool)
}

// BackendHealthState classifies backend availability and recovery status.
type BackendHealthState string

const (
	BackendHealthReady      BackendHealthState = "ready"
	BackendHealthDegraded   BackendHealthState = "degraded"
	BackendHealthUnhealthy  BackendHealthState = "unhealthy"
	BackendHealthRecovering BackendHealthState = "recovering"
)

// HealthReport captures the latest backend status snapshot.
type HealthReport struct {
	State       BackendHealthState `json:"state"`
	Message     string             `json:"message,omitempty"`
	LastError   string             `json:"last_error,omitempty"`
	LastErrorAt time.Time          `json:"last_error_at,omitempty"`
	ErrorCount  int64              `json:"error_count,omitempty"`
	UptimeSince time.Time          `json:"uptime_since,omitempty"`
	Resources   *ResourceSnapshot  `json:"resources,omitempty"`
}

// ModelInfo summarizes a backend-visible model entry.
type ModelInfo struct {
	Name          string `json:"name"`
	Family        string `json:"family,omitempty"`
	ParameterSize string `json:"parameter_size,omitempty"`
	ContextSize   int    `json:"context_size,omitempty"`
	Quantization  string `json:"quantization,omitempty"`
	HasGPU        bool   `json:"has_gpu,omitempty"`
}

// ResourceSnapshot captures coarse backend resource metrics.
type ResourceSnapshot struct {
	VRAMUsedMB      int64 `json:"vram_used_mb,omitempty"`
	VRAMTotalMB     int64 `json:"vram_total_mb,omitempty"`
	SystemRAMUsedMB int64 `json:"system_ram_used_mb,omitempty"`
	ThreadsActive   int   `json:"threads_active,omitempty"`
	KVCacheSlots    int   `json:"kv_cache_slots,omitempty"`
	KVCacheUsed     int   `json:"kv_cache_used,omitempty"`
	ModelLoaded     bool  `json:"model_loaded,omitempty"`
}
