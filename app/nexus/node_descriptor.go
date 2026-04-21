package main

import "codeburg.org/lexbit/relurpify/framework/core"

func nodeDescriptorFromEnrollment(enrollment core.NodeEnrollment) core.NodeDescriptor {
	return core.NodeDescriptor{
		ID:         enrollment.NodeID,
		TenantID:   enrollment.TenantID,
		Name:       enrollment.NodeID,
		Platform:   core.NodePlatformHeadless,
		TrustClass: enrollment.TrustClass,
		PairedAt:   enrollment.PairedAt,
		Owner:      enrollment.Owner,
	}
}
