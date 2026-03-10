package bootstrap

import (
	"time"

	nexuscfg "github.com/lexcodex/relurpify/app/nexus/config"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory/db"
	fwnode "github.com/lexcodex/relurpify/framework/middleware/node"
)

func ResolveConfig(workspace, configPath string) (config.Paths, nexuscfg.Config, error) {
	paths := config.New(workspace)
	if configPath == "" {
		configPath = paths.NexusConfigFile()
	}
	cfg, err := nexuscfg.Load(configPath)
	if err != nil {
		return paths, nexuscfg.Config{}, err
	}
	return paths, cfg, nil
}

func OpenNodeManager(paths config.Paths, cfg nexuscfg.Config) (*fwnode.Manager, *db.SQLiteNodeStore, *db.SQLiteEventLog, error) {
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

func DefaultNodeDescriptor(deviceID string) core.NodeDescriptor {
	return core.NodeDescriptor{
		ID:         deviceID,
		Name:       deviceID,
		Platform:   core.NodePlatformHeadless,
		TrustClass: core.TrustClassWorkspaceTrusted,
		PairedAt:   time.Now().UTC(),
	}
}
