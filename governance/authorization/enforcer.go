package authorization

import (
	"context"

	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
)

// Enforcer is the governance-owned authorization surface. All runtimes
// (execution, toolcapabilities, TUI) call Check. Implementations MUST be
// safe for concurrent Check and MUST NOT perform I/O.
type Enforcer interface {
	Check(ctx context.Context, req governanceports.AccessRequest) governanceports.Decision
}
