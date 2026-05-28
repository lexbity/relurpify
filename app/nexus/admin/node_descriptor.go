package admin

import (
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/relurpnet"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"codeburg.org/lexbit/relurpify/relurpnet/node"
)

func nodeDescriptorFromEnrollment(enrollment identity.NodeEnrollment) node.NodeDescriptor {
	return node.NodeDescriptor{
		ID:         enrollment.NodeID,
		TenantID:   enrollment.TenantID,
		Name:       enrollment.NodeID,
		Platform:   relurpnet.NodePlatformHeadless,
		TrustClass: agentspec.TrustClass(enrollment.TrustClass),
		PairedAt:   enrollment.PairedAt,
		Owner:      enrollment.Owner,
	}
}
