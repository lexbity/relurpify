package euclo

import (
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/orchestrate"
)

func init() {
	// Set up the snapshot function for orchestrate package
	orchestrate.SetDefaultSnapshotFunc(func(reg interface{}) euclotypes.CapabilitySnapshot {
		if registry, ok := reg.(*capability.Registry); ok {
			return snapshotCapabilities(registry)
		}
		return euclotypes.CapabilitySnapshot{}
	})
}
