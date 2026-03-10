package status

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	nexuscfg "github.com/lexcodex/relurpify/app/nexus/config"
	relconfig "github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/middleware/node"
	"gopkg.in/yaml.v3"
)

func TestLoadSummarizesGatewayState(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	paths := relconfig.New(workspace)
	if err := os.MkdirAll(paths.ConfigRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	configData, err := yaml.Marshal(nexuscfg.Config{
		Gateway: nexuscfg.GatewayConfig{
			Bind: ":9999",
			Path: "/gateway",
		},
		Channels: map[string]map[string]any{
			"webchat": {"enabled": true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.NexusConfigFile(), configData, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	eventLog, err := db.NewSQLiteEventLog(paths.EventsFile())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eventLog.Close() })
	_, err = eventLog.Append(ctx, "local", []core.FrameworkEvent{
		{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionCreated,
			Actor:     core.EventActor{Kind: "agent", ID: "sess-1"},
		},
		{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
			Payload:   []byte(`{"channel":"webchat"}`),
			Actor:     core.EventActor{Kind: "channel", ID: "webchat"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	nodeStore, err := db.NewSQLiteNodeStore(paths.NodesFile())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nodeStore.Close() })
	if err := nodeStore.UpsertNode(ctx, core.NodeDescriptor{
		ID:         "node-1",
		Name:       "Node One",
		Platform:   core.NodePlatformLinux,
		TrustClass: core.TrustClassWorkspaceTrusted,
		PairedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := nodeStore.SavePendingPairing(ctx, node.PendingPairing{
		Code: "pair-1",
		Cred: core.NodeCredential{
			DeviceID:  "pending-1",
			PublicKey: []byte("pk"),
			IssuedAt:  time.Now().UTC(),
		},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Load(ctx, workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ConfigPath != paths.NexusConfigFile() {
		t.Fatalf("ConfigPath() = %q", snapshot.ConfigPath)
	}
	if snapshot.GatewayBind != ":9999" {
		t.Fatalf("GatewayBind() = %q", snapshot.GatewayBind)
	}
	if snapshot.LastSeq == 0 {
		t.Fatal("expected LastSeq to be populated")
	}
	if snapshot.ActiveSessions != 1 {
		t.Fatalf("ActiveSessions() = %d", snapshot.ActiveSessions)
	}
	if snapshot.ObservedChannels != 1 {
		t.Fatalf("ObservedChannels() = %d", snapshot.ObservedChannels)
	}
	if len(snapshot.ConfiguredChannelIDs) != 1 || snapshot.ConfiguredChannelIDs[0] != "webchat" {
		t.Fatalf("ConfiguredChannelIDs() = %+v", snapshot.ConfiguredChannelIDs)
	}
	if snapshot.PairedNodeCount != 1 {
		t.Fatalf("PairedNodeCount() = %d", snapshot.PairedNodeCount)
	}
	if snapshot.PendingPairingCount != 1 {
		t.Fatalf("PendingPairingCount() = %d", snapshot.PendingPairingCount)
	}
	if len(snapshot.PendingPairings) != 1 || snapshot.PendingPairings[0].Code != "pair-1" {
		t.Fatalf("PendingPairings() = %+v", snapshot.PendingPairings)
	}
	if len(snapshot.PairedNodes) != 1 || snapshot.PairedNodes[0].ID != "node-1" {
		t.Fatalf("PairedNodes() = %+v", snapshot.PairedNodes)
	}
	if got := snapshot.ChannelActivity["webchat"].Inbound; got != 1 {
		t.Fatalf("ChannelActivity[webchat].Inbound = %d", got)
	}
	if snapshot.EventTypeCounts[core.FrameworkEventSessionCreated] != 1 {
		t.Fatalf("EventTypeCounts(session.created) = %d", snapshot.EventTypeCounts[core.FrameworkEventSessionCreated])
	}
	if len(snapshot.SecurityWarnings) == 0 {
		t.Fatal("expected SecurityWarnings to be populated")
	}
}

func TestLoadUsesExplicitConfigPath(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	paths := relconfig.New(workspace)
	if err := os.MkdirAll(paths.ConfigRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	customConfig := filepath.Join(workspace, "custom-nexus.yaml")
	data, err := yaml.Marshal(nexuscfg.Config{Gateway: nexuscfg.GatewayConfig{Bind: ":8123", Path: "/x"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(customConfig, data, 0o644); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Load(context.Background(), workspace, customConfig)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ConfigPath != customConfig {
		t.Fatalf("ConfigPath() = %q", snapshot.ConfigPath)
	}
	if snapshot.GatewayBind != ":8123" {
		t.Fatalf("GatewayBind() = %q", snapshot.GatewayBind)
	}
}

func TestLoadFiltersExpiredPendingPairings(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	paths := relconfig.New(workspace)
	if err := os.MkdirAll(paths.ConfigRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	configData, err := yaml.Marshal(nexuscfg.Config{
		Gateway: nexuscfg.GatewayConfig{
			Bind: ":9999",
			Path: "/gateway",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.NexusConfigFile(), configData, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	nodeStore, err := db.NewSQLiteNodeStore(paths.NodesFile())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nodeStore.Close() })
	if err := nodeStore.SavePendingPairing(ctx, node.PendingPairing{
		Code: "expired",
		Cred: core.NodeCredential{
			DeviceID:  "stale-device",
			PublicKey: []byte("pk-stale"),
			IssuedAt:  time.Now().UTC().Add(-2 * time.Hour),
		},
		ExpiresAt: time.Now().UTC().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := nodeStore.SavePendingPairing(ctx, node.PendingPairing{
		Code: "active",
		Cred: core.NodeCredential{
			DeviceID:  "live-device",
			PublicKey: []byte("pk-live"),
			IssuedAt:  time.Now().UTC(),
		},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Load(ctx, workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.PendingPairingCount != 1 {
		t.Fatalf("PendingPairingCount() = %d", snapshot.PendingPairingCount)
	}
	if len(snapshot.PendingPairings) != 1 || snapshot.PendingPairings[0].Code != "active" {
		t.Fatalf("PendingPairings() = %+v", snapshot.PendingPairings)
	}
}
