package ollama

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	llm "codeburg.org/lexbit/relurpify/platform/llm"
)

// execCommandContext and sleepFn are dependency-injection seams for tests.
var (
	execCommandContext = exec.CommandContext //nolint:gochecknoglobals // dependency-injection seam replaced by tests
	sleepFn            = time.Sleep          //nolint:gochecknoglobals // dependency-injection seam replaced by tests
)

func init() {
	llm.RegisterKind("ollama", func(cfg llm.ProviderConfig, secrets llm.ProviderSecrets) (llm.ManagedBackend, error) {
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		return NewBackend(Config{
			Endpoint:          cfg.Endpoint,
			Model:             cfg.Model,
			ModelPath:         cfg.ModelPath,
			Timeout:           cfg.Timeout,
			NativeToolCalling: cfg.NativeToolCalling,
			Debug:             cfg.Debug,
			Config:            cfg.Config,
		}, secrets.APIKey), nil
	})
}

var (
	_ llm.ManagedBackend  = (*Backend)(nil)
	_ llm.PullableBackend = (*Backend)(nil)
)

// Reset applies a reset strategy to the local ollama installation:
// "model" unloads the configured model from VRAM, "server" restarts the
// ollama service. Empty and "none" are no-ops; unknown strategies are
// ignored safely.
func (b *Backend) Reset(ctx context.Context, strategy string) error {
	strategy = strings.ToLower(strings.TrimSpace(strategy))
	if strategy == "" || strategy == "none" {
		return nil
	}

	ollamaPath, err := exec.LookPath("ollama")
	if err != nil {
		return err
	}

	switch strategy {
	case "model":
		model := strings.TrimSpace(b.cfg.Model)
		if model == "" {
			return nil
		}
		cmd := execCommandContext(ctx, ollamaPath, "stop", filepath.Clean(model))
		_ = cmd.Run()
		sleepFn(200 * time.Millisecond)
		return nil
	case "server":
		systemctlPath, err := exec.LookPath("systemctl")
		if err != nil {
			return err
		}
		cmd := execCommandContext(ctx, systemctlPath, "restart", "ollama")
		err = cmd.Run()
		if err != nil {
			// Fallback: try to stop the model if systemctl fails
			model := strings.TrimSpace(b.cfg.Model)
			if model != "" {
				_ = execCommandContext(ctx, ollamaPath, "stop", filepath.Clean(model)).Run()
			}
		}
		sleepFn(500 * time.Millisecond)
		return nil
	default:
		return nil
	}
}
