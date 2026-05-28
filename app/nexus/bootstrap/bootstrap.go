package bootstrap

import (
	"path/filepath"
	"time"

	nexuscfg "codeburg.org/lexbit/relurpify/app/nexus/config"
	"codeburg.org/lexbit/relurpify/app/nexus/db"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/relurpnet"
	fwnode "codeburg.org/lexbit/relurpify/relurpnet/node"
)

func ResolveConfig(workspace, configPath string) (cfgload.Paths, nexuscfg.Config, error) {
	paths := cfgload.New(workspace)
	if configPath == "" {
		configPath = filepath.Join(paths.ConfigRoot(), "nexus.yaml")
	}
	cfg, err := nexuscfg.Load(configPath)
	if err != nil {
		return paths, nexuscfg.Config{}, err
	}
	return paths, cfg, nil
}

func OpenNodeManager(paths cfgload.Paths, cfg nexuscfg.Config) (*fwnode.Manager, *db.SQLiteNodeStore, *db.SQLiteEventLog, error) {
	store, err := db.NewSQLiteNodeStore(paths.NodesFile())
	if err != nil {
		return nil, nil, nil, err
	}
	logStore, err := db.NewSQLiteEventLog(paths.EventsFile())
	if err != nil {
		_ = store.Close()
		return nil, nil, nil, err
	}
	manager := &fwnode.Manager{
		Store: store,
		Log:   logStore,
		Pairing: fwnode.PairingConfig{
			AutoApproveLocal: cfg.Nodes.AutoApproveLocal,
			PairingCodeTTL:   cfg.Nodes.PairingCodeTTL,
		},
	}
	return manager, store, logStore, nil
}

func DefaultNodeDescriptor(deviceID string) relurpnet.NodeDescriptor {
	return relurpnet.NodeDescriptor{
		ID:         deviceID,
		Name:       deviceID,
		Platform:   relurpnet.NodePlatformHeadless,
		TrustClass: agentspec.TrustClassWorkspaceTrusted,
		PairedAt:   time.Now().UTC(),
	}
}
