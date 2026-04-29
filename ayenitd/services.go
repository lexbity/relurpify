// services.go contains ayenitd-specific service type aliases.
// The canonical types are defined in framework/agentenv/.
package ayenitd

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
)

// Service is a type alias to agentenv.Service.
type Service = agentenv.Service

// ServiceManager is a type alias to agentenv.ServiceManager.
type ServiceManager = agentenv.ServiceManager

// NewServiceManager is a wrapper for agentenv.NewServiceManager.
func NewServiceManager() *ServiceManager {
	return agentenv.NewServiceManager()
}
