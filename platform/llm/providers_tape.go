package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const tapeProviderName = "tape"

func init() {
	RegisterKind(tapeProviderName, func(cfg ProviderConfig, secrets ProviderSecrets) (ManagedBackend, error) {
		_ = secrets
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		inspection, err := InspectTape(cfg.TapePath)
		if err != nil {
			return nil, fmt.Errorf("inspect tape %q: %w", cfg.TapePath, err)
		}
		modelName := strings.TrimSpace(cfg.Model)
		if modelName == "" && inspection.Header != nil {
			modelName = strings.TrimSpace(inspection.Header.ModelName)
		}
		if modelName == "" {
			return nil, errors.New("provider \"" + tapeProviderName + "\" model required")
		}
		tapeModel, err := NewTapeModel(nil, cfg.TapePath, string(TapeReplay))
		if err != nil {
			return nil, fmt.Errorf("open tape model %q: %w", cfg.TapePath, err)
		}
		header := TapeHeader{
			ProviderID: tapeProviderName,
			ModelName:  modelName,
		}
		if inspection.Header != nil {
			header.ModelDigest = inspection.Header.ModelDigest
			header.FrameworkVersion = inspection.Header.FrameworkVersion
			header.RecordedAt = inspection.Header.RecordedAt
			header.SuiteName = inspection.Header.SuiteName
			header.CaseName = inspection.Header.CaseName
		}
		if err := tapeModel.ConfigureHeader(TapeHeader{
			ProviderID:       header.ProviderID,
			ModelName:        header.ModelName,
			ModelDigest:      header.ModelDigest,
			FrameworkVersion: header.FrameworkVersion,
			RecordedAt:       header.RecordedAt,
			SuiteName:        header.SuiteName,
			CaseName:         header.CaseName,
		}); err != nil {
			return nil, fmt.Errorf("validate tape header %q: %w", cfg.TapePath, err)
		}
		return tapeBackendAdapter{
			model:     tapeModel,
			modelName: modelName,
		}, nil
	})
}

type tapeBackendAdapter struct {
	model     *TapeModel
	modelName string
}

func (a tapeBackendAdapter) Model() LanguageModel {
	return a.model
}

func (a tapeBackendAdapter) Embedder() Embedder {
	return nil
}

func (a tapeBackendAdapter) Capabilities() BackendCapabilities {
	return BackendCapabilities{
		Streaming:            true,
		BackendClass:         BackendClassTransport,
		ContextSizeDiscovery: false,
	}
}

func (a tapeBackendAdapter) ModelContextSize(context.Context) (int, error) {
	return 0, nil
}

func (a tapeBackendAdapter) Health(context.Context) (*HealthReport, error) {
	return &HealthReport{
		State:   BackendHealthReady,
		Message: tapeProviderName + " replay ready",
	}, nil
}

func (a tapeBackendAdapter) ListModels(context.Context) ([]ModelInfo, error) {
	if strings.TrimSpace(a.modelName) == "" {
		return nil, nil
	}
	return []ModelInfo{{Name: a.modelName}}, nil
}

func (a tapeBackendAdapter) Warm(context.Context) error {
	return nil
}

func (a tapeBackendAdapter) Close() error {
	if a.model == nil {
		return nil
	}
	return a.model.Close()
}

func (a tapeBackendAdapter) SetDebugLogging(bool) {}

func (a tapeBackendAdapter) SetProfile(*ModelProfile) {}

func (a tapeBackendAdapter) Reset(context.Context, string) error {
	return nil
}

var _ ManagedBackend = tapeBackendAdapter{}
