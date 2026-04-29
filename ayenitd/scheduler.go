// scheduler.go contains ayenitd-specific scheduler type aliases.
// The canonical types are defined in framework/agentenv/.
package ayenitd

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
)

// ScheduledJob is a type alias to agentenv.ScheduledJob.
type ScheduledJob = agentenv.ScheduledJob

// ServiceScheduler is a type alias to agentenv.ServiceScheduler.
type ServiceScheduler = agentenv.ServiceScheduler

// NewServiceScheduler is a wrapper for agentenv.NewServiceScheduler.
func NewServiceScheduler() *ServiceScheduler {
	return agentenv.NewServiceScheduler()
}
