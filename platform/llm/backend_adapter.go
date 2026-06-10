package llm

import (
	"context"
	"os/exec"
	"strings"
	"time"

	ollamabackend "codeburg.org/lexbit/relurpify/platform/llm/ollama"
)

var (
	execCommandContext = exec.CommandContext
	sleepFn            = time.Sleep
)

type managedBackendAdapter struct {
	inner     *ollamabackend.Backend
	modelName string
}

func (a managedBackendAdapter) Model() LanguageModel {
	return a.inner.Model()
}

func (a managedBackendAdapter) Embedder() Embedder {
	return a.inner.Embedder()
}

func (a managedBackendAdapter) Capabilities() BackendCapabilities {
	caps := a.inner.Capabilities()
	return BackendCapabilities(caps)
}

func (a managedBackendAdapter) ModelContextSize(ctx context.Context) (int, error) {
	if a.inner == nil {
		return 0, nil
	}
	return a.inner.ModelContextSize(ctx)
}

func (a managedBackendAdapter) Health(ctx context.Context) (*HealthReport, error) {
	report, err := a.inner.Health(ctx)
	if report == nil {
		return nil, err
	}
	return &HealthReport{
		State:       BackendHealthState(report.State),
		Message:     report.Message,
		LastError:   report.LastError,
		LastErrorAt: report.LastErrorAt,
		ErrorCount:  report.ErrorCount,
		UptimeSince: report.UptimeSince,
		Resources:   nil,
	}, err
}

func (a managedBackendAdapter) ListModels(ctx context.Context) ([]ModelInfo, error) {
	models, err := a.inner.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ModelInfo, len(models))
	for i, model := range models {
		out[i] = ModelInfo{
			Name:          model.Name,
			Family:        model.Family,
			ParameterSize: model.ParameterSize,
			ContextSize:   model.ContextSize,
			Quantization:  model.Quantization,
			HasGPU:        model.HasGPU,
		}
	}
	return out, nil
}

func (a managedBackendAdapter) Warm(ctx context.Context) error {
	return a.inner.Warm(ctx)
}

func (a managedBackendAdapter) Close() error {
	return a.inner.Close()
}

func (a managedBackendAdapter) SetDebugLogging(enabled bool) {
	a.inner.SetDebugLogging(enabled)
}

func (a managedBackendAdapter) SetProfile(profile *ModelProfile) {
	if a.inner == nil {
		return
	}
	a.inner.SetProfile(profile)
}

func (a managedBackendAdapter) Reset(ctx context.Context, strategy string) error {
	strategy = strings.ToLower(strings.TrimSpace(strategy))
	if strategy == "" || strategy == "none" {
		return nil
	}

	switch strategy {
	case "model":
		// Unload the specific model from VRAM
		model := strings.TrimSpace(a.modelName)
		if model == "" {
			return nil
		}
		cmd := execCommandContext(ctx, "ollama", "stop", model)
		_ = cmd.Run()
		sleepFn(200 * time.Millisecond)
		return nil
	case "server":
		// Restart the Ollama service
		cmd := execCommandContext(ctx, "systemctl", "restart", "ollama")
		err := cmd.Run()
		if err != nil {
			// Fallback: try to stop the model if systemctl fails
			model := strings.TrimSpace(a.modelName)
			if model != "" {
				_ = execCommandContext(ctx, "ollama", "stop", model).Run()
			}
		}
		sleepFn(500 * time.Millisecond)
		return nil
	default:
		// Unknown strategy - ignore safely
		return nil
	}
}

func (a managedBackendAdapter) Pull(ctx context.Context, model string) error {
	if a.inner == nil {
		return nil
	}
	return a.inner.Pull(ctx, model)
}

var _ ManagedBackend = managedBackendAdapter{}
var _ PullableBackend = managedBackendAdapter{}
