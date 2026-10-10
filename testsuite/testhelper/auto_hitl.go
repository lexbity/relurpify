package testhelper

import (
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/policy"
)

// HITLApprover is the minimal runtime surface needed to answer interactive
// permission requests: the same shape the TUI's HITL service exposes.
type HITLApprover interface {
	SubscribeHITL() (<-chan authorization.HITLEvent, func())
	ApproveHITL(requestID, approver string, scope policy.GrantScope, duration time.Duration) error
}

// AutoApproveHITL answers every pending HITL permission request with an
// approval for the duration of the test — the explicit test-side approver for
// runtimes booted with the production broker (the runtime itself never
// auto-approves). The returned func unsubscribes and waits for the approver
// goroutine to exit.
func AutoApproveHITL(t *testing.T, rt HITLApprover) func() {
	t.Helper()

	ch, cancel := rt.SubscribeHITL()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range ch {
			if ev.Request == nil {
				continue
			}
			SafeApproveHITL(rt, ev.Request.ID)
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

// SafeApproveHITL approves one request, tolerating a broker that shut down
// mid-request (boot races) — an approval failure on a dead runtime is not a
// test failure.
func SafeApproveHITL(rt HITLApprover, requestID string) {
	defer func() { _ = recover() }()
	_ = rt.ApproveHITL(requestID, "test-approver", "", 0)
}
