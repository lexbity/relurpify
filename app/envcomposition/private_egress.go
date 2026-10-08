package envcomposition

import (
	"context"
	"fmt"
	"strings"

	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/platform/tools/subprocess"
)

// privateEgressApprover bridges the governance HITL broker to the subprocess
// egress port. It lives at the composition root because it is the only layer
// that may couple the platform egress scanner to governance authorization.
type privateEgressApprover struct {
	manager *fauthorization.PermissionManager
}

// NewPrivateEgressApprover returns the subprocess egress approval port backed
// by the supplied permission manager. A nil manager yields a closed approver
// (all private egress denied) rather than a permissive one.
func NewPrivateEgressApprover(manager *fauthorization.PermissionManager) subprocess.PrivateEgressApprover {
	return privateEgressApprover{manager: manager}
}

// ApprovePrivateEgress obtains (or reuses) a session grant for each host. The
// permission manager's grant cache short-circuits repeated requests, so the
// human is prompted once per host per session.
func (a privateEgressApprover) ApprovePrivateEgress(ctx context.Context, agentID string, hosts []string) error {
	if a.manager == nil {
		return fmt.Errorf("permission manager unavailable")
	}
	for _, host := range hosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host == "" {
			continue
		}
		if err := a.manager.RequireApproval(ctx, agentID, permissions.PermissionDescriptor{
			Type:         permissions.PermissionTypeNetwork,
			Action:       "net-egress-private:" + host,
			Resource:     host,
			RequiresHITL: true,
		}, "private network egress requires approval", policy.GrantScopeSession, policy.RiskLevelHigh, 0); err != nil {
			return err
		}
	}
	return nil
}
