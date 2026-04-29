package fmp

import (
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

func subjectRefFromDelegation(ref core.DelegationSubjectRef) identity.SubjectRef {
	return identity.SubjectRef{
		TenantID: strings.TrimSpace(ref.TenantID),
		Kind:     identity.SubjectKind(strings.TrimSpace(ref.Kind)),
		ID:       strings.TrimSpace(ref.ID),
	}
}

func subjectRefsEqual(left, right identity.SubjectRef) bool {
	return strings.EqualFold(left.TenantID, right.TenantID) &&
		strings.EqualFold(string(left.Kind), string(right.Kind)) &&
		strings.EqualFold(left.ID, right.ID)
}
