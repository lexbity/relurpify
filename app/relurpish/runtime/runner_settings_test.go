package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func TestRunnerSettingsFor_MissingConfigDefaults(t *testing.T) {
	// A missing config file is non-blocking: template defaults apply —
	// runner enabled, knowledge queue, refresh off (absent, not stubbed).
	settings, err := runnerSettingsFor(filepath.Join(t.TempDir(), "absent", "relurpify.yaml"))
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 2, settings.Workers)
	require.Equal(t, []string{"knowledge"}, settings.Queues)
	require.Equal(t, 500*time.Millisecond, settings.PollInterval)
	require.Equal(t, 5*time.Second, settings.Heartbeat)
	require.Equal(t, time.Duration(0), settings.RefreshInterval)
}

func TestRunnerSettingsFor_AuthoredSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relurpify_cfg", "workspace.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(`schema: relurpify/workspace/v1
paths:
  state_dir: .relurpify_state
model:
  provider: ollama
  name: test-model
sandbox:
  backend: gvisor
logging:
  level: info
  format: json
audit:
  retention_days: 7
  enforcement: strict
telemetry:
  enabled: false
runner:
  enabled: true
  workers: 4
  queues:
    - knowledge
    - maintenance
  poll_interval: 250ms
  heartbeat: 2s
  refresh_interval: 90s
`), 0o600))

	settings, err := runnerSettingsFor(path)
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 4, settings.Workers)
	require.Equal(t, []string{"knowledge", "maintenance"}, settings.Queues)
	require.Equal(t, 250*time.Millisecond, settings.PollInterval)
	require.Equal(t, 2*time.Second, settings.Heartbeat)
	require.Equal(t, 90*time.Second, settings.RefreshInterval)
}

func TestRunnerSettingsFor_ExplicitDisableHonored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workspace.yaml")
	base := `schema: relurpify/workspace/v1
paths:
  state_dir: .relurpify_state
model:
  provider: ollama
  name: test-model
sandbox:
  backend: gvisor
logging:
  level: info
  format: json
audit:
  retention_days: 7
  enforcement: strict
telemetry:
  enabled: false
runner:
  enabled: false
`
	require.NoError(t, os.WriteFile(path, []byte(base), 0o600))
	settings, err := runnerSettingsFor(path)
	require.NoError(t, err)
	require.False(t, settings.Enabled, "an authored enabled: false disables the runner")
}

func TestRunnerConfigResolved_Validation(t *testing.T) {
	// Bad durations surface as named field errors, not silent defaults.
	_, err := config.RunnerConfigV1{PollInterval: "soon"}.Resolved()
	require.ErrorContains(t, err, "runner.poll_interval")
	_, err = config.RunnerConfigV1{Heartbeat: "-5s"}.Resolved()
	require.ErrorContains(t, err, "runner.heartbeat")
	_, err = config.RunnerConfigV1{RefreshInterval: "forever"}.Resolved()
	require.ErrorContains(t, err, "runner.refresh_interval")
}
