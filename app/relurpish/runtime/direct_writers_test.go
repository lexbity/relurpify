package runtime

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/telemetry/otelleg"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestDirectWritersConverted proves D10: the three former direct framework-log
// writers are now bus events that reach the OTLP-JSON file leg with their
// metadata preserved. The document reload and contract resolution are point
// events (log records); the agent startup opens the session-lifetime agent span.
func TestDirectWritersConverted(t *testing.T) {
	dir := t.TempDir()
	chain, err := otelleg.New(context.Background(), otelleg.Settings{Enabled: true, FileEnabled: true, FileDir: dir})
	require.NoError(t, err)

	snapshot := &config.DocumentSnapshot{
		SourcePath:  "/ws/relurpify_cfg/AGENTS.md",
		Fingerprint: [32]byte{0xde, 0xad, 0xbe, 0xef},
		Warnings:    []string{"warning-one"},
	}
	ctx := context.Background()
	emitDocumentReloadedEvent(ctx, chain, "euclo", "euclo", snapshot)
	emitAgentStartupEvent(ctx, chain, "euclo", "euclo", nil)
	emitContractResolvedEvent(ctx, chain, "euclo", "euclo", snapshot)
	require.NoError(t, chain.Close(ctx))

	segment := otlpSegmentFile(t, dir)
	raw, err := os.ReadFile(segment)
	require.NoError(t, err)
	body := string(raw)

	// The two point events are log records with their old payload fields.
	require.Contains(t, body, "context_policy_reloaded")
	require.Contains(t, body, "document_reloaded")
	require.Contains(t, body, "document_path")
	require.Contains(t, body, "warning-one")
	require.Contains(t, body, "contract_resolved")

	// The agent startup is an agent-family span carrying phase=boot.
	require.Contains(t, body, "agent run euclo")
	require.Contains(t, body, "phase")
	require.Contains(t, body, "boot")
}
