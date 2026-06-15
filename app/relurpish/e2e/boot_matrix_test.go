package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	relurpishruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func TestBootMatrix_NoPanic(t *testing.T) {
	type matrixCell struct {
		name         string
		wantDegraded bool
	}
	offlineProvider := "offline"
	offlineModel := "offline-synthetic"

	cells := []matrixCell{
		{name: "valid_config"},
		{name: "missing_config", wantDegraded: true},
		{name: "invalid_config", wantDegraded: true},
	}

	for _, cell := range cells {
		t.Run(cell.name, func(t *testing.T) {
			workspace := t.TempDir()
			ctx := context.Background()
			secrets := config.Secrets{}

			cfg := relurpishruntime.ConfigForWorkspace(relurpishruntime.DefaultConfig(), workspace)
			cfg.InferenceProvider = offlineProvider
			cfg.InferenceModel = offlineModel
			cfg.InferenceNativeToolCalling = true
			cfg.SecurityRunner = &recordingRunner{}
			cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
				return &fakeSandboxRuntime{}, nil
			}

			switch {
			case strings.Contains(cell.name, "missing_config"):
			case strings.Contains(cell.name, "invalid_config"):
				if err := os.MkdirAll(filepath.Dir(cfg.ConfigPath), 0755); err != nil {
					t.Fatalf("mkdir config dir: %v", err)
				}
				if err := os.WriteFile(cfg.ConfigPath, []byte("schema: invalid\nbroken: true\ninvalid_field: true\n"), 0644); err != nil {
					t.Fatalf("write invalid config: %v", err)
				}
			default:
				testhelper.WriteCleanWorkspace(t, workspace, testhelper.WorkspaceOpts{
					Provider: offlineProvider,
				})
			}

			rt, err := relurpishruntime.New(ctx, cfg, secrets)
			if err != nil {
				t.Fatalf("total construction must not return error: %v", err)
			}
			if rt == nil {
				t.Fatal("runtime must not be nil")
			}
			ws := rt.AgentWorkspace()
			if ws == nil {
				t.Fatal("AgentWorkspace() must never be nil (INV-1)")
			}
			_ = rt.Close(ctx)
		})
	}
}
