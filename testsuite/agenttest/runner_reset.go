package agenttest

import (
	"context"
	"log"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/llm"
)

// resetBackendIfNeeded conditionally resets the backend based on the provided options.
// It delegates to the backend's Reset method with the appropriate strategy.
// This is a provider-agnostic replacement for the ollama-specific maybeResetBackend.
func resetBackendIfNeeded(ctx context.Context, logger *log.Logger, backend llm.ManagedBackend, opts RunOptions, modelName string) error {
	strategy := strings.ToLower(strings.TrimSpace(opts.BackendReset))
	if strategy == "" || strategy == "none" {
		return nil
	}

	if logger != nil {
		logger.Printf("resetting backend with strategy=%s model=%s", strategy, modelName)
	}

	return backend.Reset(ctx, strategy)
}
