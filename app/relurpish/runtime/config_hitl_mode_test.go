package runtime

import (
	"testing"

	"github.com/stretchr/testify/require"

	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
)

// TestNormalizeDefaultsHITLModeGoverned pins the shipped posture: an empty
// HITLMode resolves to governed.
func TestNormalizeDefaultsHITLModeGoverned(t *testing.T) {
	cfg := Config{Workspace: t.TempDir()}
	require.NoError(t, cfg.Normalize())
	require.Equal(t, fauthorization.HITLModeGoverned, cfg.HITLMode)
}

// TestNormalizeRejectsUnknownHITLMode proves an unknown posture is a
// configuration error at normalize time — never a silent governed fallback
// and never a runtime surprise mid-boot.
func TestNormalizeRejectsUnknownHITLMode(t *testing.T) {
	cfg := Config{Workspace: t.TempDir(), HITLMode: "yolo"}
	err := cfg.Normalize()
	require.Error(t, err)
	require.Contains(t, err.Error(), "hitl mode")
}

// TestNormalizeAcceptsPermissiveHITLMode proves the permissive posture parses
// (the explicit operator posture, --hitl-mode permissive).
func TestNormalizeAcceptsPermissiveHITLMode(t *testing.T) {
	cfg := Config{Workspace: t.TempDir(), HITLMode: fauthorization.HITLModePermissive}
	require.NoError(t, cfg.Normalize())
	require.Equal(t, fauthorization.HITLModePermissive, cfg.HITLMode)
}

// TestProbeHITLMode proves the doctor surfaces both postures and degrades on
// permissive so the posture is impossible to miss.
func TestProbeHITLMode(t *testing.T) {
	governed := probeHITLMode(Config{})
	require.Equal(t, "hitl_mode", governed.Name)
	require.True(t, governed.Available)
	require.False(t, governed.Degraded)
	require.Contains(t, governed.Details, "governed")

	permissive := probeHITLMode(Config{HITLMode: fauthorization.HITLModePermissive})
	require.True(t, permissive.Available)
	require.True(t, permissive.Degraded, "permissive must be doctor-visible as degraded posture")
	require.Contains(t, permissive.Details, "permissive")
}
