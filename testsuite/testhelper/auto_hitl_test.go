package testhelper

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/policy"
)

// fakeHITLApprover is a minimal HITLApprover: Subscribe delivers events from a
// slice, Approve records approvals; cancel closes the channel exactly once.
type fakeHITLApprover struct {
	events   []authorization.HITLEvent
	approved []string
	ch       chan authorization.HITLEvent
	cancel   sync.Once
}

func newFakeHITLApprover(events ...authorization.HITLEvent) *fakeHITLApprover {
	return &fakeHITLApprover{
		events: events,
		ch:     make(chan authorization.HITLEvent, len(events)+1),
	}
}

func (f *fakeHITLApprover) SubscribeHITL() (<-chan authorization.HITLEvent, func()) {
	for _, ev := range f.events {
		f.ch <- ev
	}
	return f.ch, func() { f.cancel.Do(func() { close(f.ch) }) }
}

func (f *fakeHITLApprover) ApproveHITL(requestID, approver string, _ policy.GrantScope, _ time.Duration) error {
	f.approved = append(f.approved, requestID+":"+approver)
	return nil
}

// panickingApprover fails hard on approve to prove SafeApproveHITL survives.
type panickingApprover struct{}

func (panickingApprover) SubscribeHITL() (<-chan authorization.HITLEvent, func()) {
	ch := make(chan authorization.HITLEvent)
	close(ch)
	return ch, func() {}
}

func (panickingApprover) ApproveHITL(string, string, policy.GrantScope, time.Duration) error {
	panic("broker exploded")
}

func TestAutoApproveHITLApprovesEveryPendingRequest(t *testing.T) {
	rt := newFakeHITLApprover(
		authorization.HITLEvent{Type: authorization.HITLEventRequested, Request: &authorization.PermissionRequest{ID: "req-1"}},
		authorization.HITLEvent{Type: authorization.HITLEventRequested, Request: &authorization.PermissionRequest{ID: "req-2"}},
		// Events without a request payload are skipped, not approved.
		authorization.HITLEvent{Type: authorization.HITLEventResolved},
	)

	stop := AutoApproveHITL(t, rt)
	stop()

	assert.ElementsMatch(t, []string{"req-1:test-approver", "req-2:test-approver"}, approvedRequests(rt))
}

// approvedRequests snapshots the approvals after the approver goroutine exited.
func approvedRequests(rt *fakeHITLApprover) []string {
	return rt.approved
}

func TestSafeApproveHITLToleratesPanicAndFailure(t *testing.T) {
	assert.NotPanics(t, func() { SafeApproveHITL(panickingApprover{}, "req-1") })
	assert.NotPanics(t, func() { SafeApproveHITL(failingApprover{}, "req-2") })
}

// failingApprover returns an error from every approval.
type failingApprover struct{}

func (failingApprover) SubscribeHITL() (<-chan authorization.HITLEvent, func()) {
	ch := make(chan authorization.HITLEvent)
	close(ch)
	return ch, func() {}
}

func (failingApprover) ApproveHITL(string, string, policy.GrantScope, time.Duration) error {
	return assert.AnError
}

func TestAutoApproveHITLStopIsIdempotentOnQuietBroker(t *testing.T) {
	rt := newFakeHITLApprover()
	stop := AutoApproveHITL(t, rt)
	stop()
	// A second stop on an already-drained approver must not hang or panic.
	require.NotPanics(t, stop)
	assert.Empty(t, approvedRequests(rt))
}
