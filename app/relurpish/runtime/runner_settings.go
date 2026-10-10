// runner_settings.go resolves the workspace config's runner section for the
// runtime (S9). Missing config file: defaults with the runner enabled — the
// template default is the product default.
package runtime

import (
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// runnerSettingsFor resolves the runner section from the workspace V1
// config. A missing or unreadable file yields the template defaults (the
// same Resolved the decoder applies): runner enabled, 2 workers, knowledge
// queue, 500 ms poll, 5 s heartbeat, refresh off.
func runnerSettingsFor(configPath string) (config.RunnerSettings, error) {
	var section config.RunnerConfigV1
	if configPath != "" {
		v1, err := config.LoadRuntimeWorkspaceConfigV1(configPath)
		if err == nil {
			section = v1.Runner
		}
		// A missing file is non-blocking (uninitialized workspace); a
		// present-but-invalid file is handled by the blocking V1 load
		// earlier in boot — here the defaults apply.
	}
	return section.Resolved()
}
