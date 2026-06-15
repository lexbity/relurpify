package runtime

// ConfigState describes the parse/validation status of the on-disk workspace config.
type ConfigState string

const (
	ConfigValid   ConfigState = "valid"
	ConfigMissing ConfigState = "missing"
	ConfigInvalid ConfigState = "invalid"
)

// Readiness captures whether the workspace is fully operational.
// Two-axis readiness (SandboxReady + ModelReady) gates tool and chat access.
type Readiness struct {
	SandboxReady bool
	ModelReady   bool
	ConfigState  ConfigState
	Degraded     bool
	Reason       string
}

// Ready returns true when the workspace is fully operational
// (sandbox verified, model healthy, config valid).
func (r Readiness) Ready() bool {
	return r.SandboxReady && r.ModelReady && r.ConfigState == ConfigValid
}
