package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/telemetry"
)

type preparedRunTelemetry struct {
	*telemetry.JSONFileTelemetry
	path string
}

func newPreparedRunTelemetry(path string) (*preparedRunTelemetry, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("telemetry path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	sink, err := telemetry.NewJSONFileTelemetry(path)
	if err != nil {
		return nil, err
	}
	return &preparedRunTelemetry{JSONFileTelemetry: sink, path: path}, nil
}

func emitPreparedRunTelemetryEvent(sink core.Telemetry, eventType, message string, metadata map[string]any) {
	if sink == nil {
		return
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	sink.Emit(core.Event{
		Type:      core.EventType(eventType),
		Timestamp: time.Now().UTC(),
		Message:   message,
		Metadata:  metadata,
	})
}

func (t *preparedRunTelemetry) Close() error {
	if t == nil || t.JSONFileTelemetry == nil {
		return nil
	}
	return t.JSONFileTelemetry.Close()
}

func preparedRunTelemetryMetadata(desc *preparedRunWorkspaceTarget) map[string]any {
	if desc == nil || desc.Descriptor == nil {
		return nil
	}
	return map[string]any{
		"run_id":                desc.Descriptor.RunID,
		"workspace":             desc.Config.Workspace,
		"backend_provider":      desc.Descriptor.BackendProvider,
		"backend_family":        desc.Descriptor.BackendFamily,
		"backend_endpoint":      desc.Descriptor.BackendEndpoint,
		"backend_binary":        desc.Descriptor.BackendBinary,
		"backend_service":       desc.Descriptor.BackendService,
		"service_reset":         desc.Descriptor.ServiceResetStrategy,
		"service_reset_between": desc.Descriptor.ServiceResetBetween,
	}
}

func preparedRunSetupLogMessage(desc *preparedRunWorkspaceTarget) string {
	if desc == nil || desc.Descriptor == nil {
		return "prepared run setup"
	}
	return fmt.Sprintf("prepared run setup run_id=%s workspace=%s provider=%s family=%s endpoint=%s binary=%s service=%s reset=%s",
		desc.Descriptor.RunID,
		desc.Config.Workspace,
		desc.Descriptor.BackendProvider,
		desc.Descriptor.BackendFamily,
		desc.Descriptor.BackendEndpoint,
		desc.Descriptor.BackendBinary,
		desc.Descriptor.BackendService,
		desc.Descriptor.ServiceResetStrategy,
	)
}

func preparedRunExecutionLogMessage(desc *preparedRunWorkspaceTarget) string {
	if desc == nil || desc.Descriptor == nil {
		return "prepared run execution"
	}
	return fmt.Sprintf("prepared run execution run_id=%s workspace=%s", desc.Descriptor.RunID, desc.Config.Workspace)
}
