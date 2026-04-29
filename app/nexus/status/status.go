package status

import (
	"context"
	"fmt"
	"sort"
	"time"

	nexuscfg "codeburg.org/lexbit/relurpify/app/nexus/config"
	"codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/app/nexus/gateway"
	"codeburg.org/lexbit/relurpify/framework/event"
	frameworkmanifest "codeburg.org/lexbit/relurpify/framework/manifest"
	fwnode "codeburg.org/lexbit/relurpify/relurpnet/node"
)

type Snapshot struct {
	Workspace            string
	ConfigPath           string
	GatewayBind          string
	GatewayPath          string
	EventsFile           string
	LastSeq              uint64
	SnapshotLoaded       bool
	ConfiguredChannels   int
	ConfiguredChannelIDs []string
	ActiveSessions       int
	ObservedChannels     int
	PairedNodeCount      int
	PendingPairingCount  int
	PairedNodes          []fwnode.NodeDescriptor
	ChannelActivity      map[string]gateway.ChannelState
	EventTypeCounts      map[string]uint64
	LogRetentionDays     int
	AutoApproveLocal     bool
	PairingCodeTTL       time.Duration
	SecurityWarnings     []string
	PendingPairings      []PendingPairingInfo
}

type PendingPairingInfo struct {
	Code      string
	DeviceID  string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

func Load(ctx context.Context, workspace, configPath string) (Snapshot, error) {
	paths := frameworkmanifest.New(workspace)
	if configPath == "" {
		configPath = paths.NexusConfigFile()
	}
	cfg, err := nexuscfg.Load(configPath)
	if err != nil {
		return Snapshot{}, err
	}
	eventLog, err := db.NewSQLiteEventLog(paths.EventsFile())
	if err != nil {
		return Snapshot{}, err
	}
	defer eventLog.Close()
	nodeStore, err := db.NewSQLiteNodeStore(paths.NodesFile())
	if err != nil {
		return Snapshot{}, err
	}
	defer nodeStore.Close()
	if _, err := nodeStore.DeleteExpiredPendingPairings(ctx, time.Now().UTC()); err != nil {
		return Snapshot{}, err
	}
	state := gateway.NewStateMaterializer()
	runner := &event.Runner{
		Log:           eventLog,
		Materializers: []event.Materializer{state},
		Partition:     "local",
	}
	if err := runner.RestoreAndRunOnce(ctx); err != nil {
		return Snapshot{}, err
	}
	_, snapshotData, err := eventLog.LoadSnapshot(ctx, "local")
	if err != nil {
		return Snapshot{}, err
	}
	pairedNodes, err := nodeStore.ListNodes(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	pendingPairings, err := nodeStore.ListPendingPairings(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	stateView := state.State()
	channelIDs := make([]string, 0, len(cfg.Channels))
	for name := range cfg.Channels {
		channelIDs = append(channelIDs, name)
	}
	sort.Strings(channelIDs)
	pendingInfo := make([]PendingPairingInfo, 0, len(pendingPairings))
	for _, pairing := range pendingPairings {
		pendingInfo = append(pendingInfo, PendingPairingInfo{
			Code:      pairing.Code,
			DeviceID:  pairing.Cred.DeviceID,
			IssuedAt:  pairing.Cred.IssuedAt,
			ExpiresAt: pairing.ExpiresAt,
		})
	}
	return Snapshot{
		Workspace:            workspace,
		ConfigPath:           configPath,
		GatewayBind:          cfg.Gateway.Bind,
		GatewayPath:          cfg.Gateway.Path,
		EventsFile:           paths.EventsFile(),
		LastSeq:              stateView.LastSeq,
		SnapshotLoaded:       len(snapshotData) > 0,
		ConfiguredChannels:   len(cfg.Channels),
		ConfiguredChannelIDs: channelIDs,
		ActiveSessions:       len(stateView.ActiveSessions),
		ObservedChannels:     len(stateView.ChannelActivity),
		PairedNodeCount:      len(pairedNodes),
		PendingPairingCount:  len(pendingPairings),
		PairedNodes:          pairedNodes,
		ChannelActivity:      stateView.ChannelActivity,
		EventTypeCounts:      stateView.EventTypeCounts,
		LogRetentionDays:     cfg.Gateway.Log.RetentionDays,
		AutoApproveLocal:     cfg.Nodes.AutoApproveLocal,
		PairingCodeTTL:       cfg.Nodes.PairingCodeTTL,
		SecurityWarnings:     cfg.SecurityWarnings(len(pendingPairings)),
		PendingPairings:      pendingInfo,
	}, nil
}

func (s Snapshot) Summary() string {
	return fmt.Sprintf(
		"Gateway bind: %s\nGateway path: %s\nEvent log: %s\nLast seq: %d\nSnapshot loaded: %t\nConfigured channels: %d\nActive sessions: %d\nObserved channels: %d\nPaired nodes: %d\nPending pairings: %d\n",
		s.GatewayBind,
		s.GatewayPath,
		s.EventsFile,
		s.LastSeq,
		s.SnapshotLoaded,
		s.ConfiguredChannels,
		s.ActiveSessions,
		s.ObservedChannels,
		s.PairedNodeCount,
		s.PendingPairingCount,
	)
}
