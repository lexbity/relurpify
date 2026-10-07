package authorization

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/governance/policy"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
)

type HITLTimeoutBehavior string

const (
	HITLTimeoutBehaviorFail HITLTimeoutBehavior = "fail"
	HITLTimeoutBehaviorSkip HITLTimeoutBehavior = "skip"
)

// PermissionRequest captures a pending permission escalation.
type PermissionRequest struct {
	ID              string                           `json:"id"`
	Permission      permissions.PermissionDescriptor `json:"permission"`
	Justification   string                           `json:"justification"`
	Scope           policy.GrantScope                `json:"scope"`
	Duration        time.Duration                    `json:"duration"`
	Risk            policy.RiskLevel                 `json:"risk"`
	RunID           string                           `json:"run_id,omitempty"`
	Timeout         time.Duration                    `json:"timeout,omitempty"`
	TimeoutBehavior HITLTimeoutBehavior              `json:"timeout_behavior,omitempty"`
	RequestedAt     time.Time                        `json:"requested_at"`
	State           string                           `json:"state"`
}

// PermissionDecision encapsulates an approval or rejection.
type PermissionDecision struct {
	RequestID  string            `json:"request_id"`
	Approved   bool              `json:"approved"`
	ApprovedBy string            `json:"approved_by"`
	Scope      policy.GrantScope `json:"scope"`
	ExpiresAt  time.Time         `json:"expires_at"`
	Reason     string            `json:"reason,omitempty"`
	Conditions map[string]string `json:"conditions,omitempty"`
}

// HITLBroker coordinates blocking and async approvals.
type HITLBroker struct {
	timeout     time.Duration
	mu          sync.Mutex
	requests    map[string]*PermissionRequest
	waiters     map[string]chan PermissionDecision
	subs        map[int]chan HITLEvent
	subSeq      int
	clock       func() time.Time
	AutoApprove bool
	decisions   fwtelemetry.DecisionSink
}

// NewHITLBroker builds a broker with the supplied timeout and the decision
// sink that receives the structured HITL lifecycle forensics (FR-6).
func NewHITLBroker(timeout time.Duration, decisions fwtelemetry.DecisionSink) *HITLBroker {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &HITLBroker{
		timeout:   timeout,
		requests:  make(map[string]*PermissionRequest),
		waiters:   make(map[string]chan PermissionDecision),
		subs:      make(map[int]chan HITLEvent),
		clock:     time.Now,
		decisions: decisions,
	}
}

// SetDecisionSink re-wires decision forensics after construction so the
// composition root can attach the full sink multiplex once telemetry is
// assembled (the broker is built before that point).
func (h *HITLBroker) SetDecisionSink(sink fwtelemetry.DecisionSink) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.decisions = sink
	h.mu.Unlock()
}

// HITLEventType describes the lifecycle stage of a HITL permission request.
type HITLEventType string

const (
	HITLEventRequested HITLEventType = "requested"
	HITLEventResolved  HITLEventType = "resolved"
	HITLEventExpired   HITLEventType = "expired"
)

// HITLEvent is emitted whenever a permission request is created, resolved, or expires.
type HITLEvent struct {
	Type     HITLEventType
	Request  *PermissionRequest
	Decision *PermissionDecision
	Error    string
}

// Subscribe returns a channel that receives HITL lifecycle events.
// Call the returned cancel function to unsubscribe.
func (h *HITLBroker) Subscribe(buffer int) (<-chan HITLEvent, func()) {
	if h == nil {
		ch := make(chan HITLEvent)
		close(ch)
		return ch, func() {}
	}
	if buffer <= 0 {
		buffer = 16
	}
	ch := make(chan HITLEvent, buffer)
	h.mu.Lock()
	id := h.subSeq
	h.subSeq++
	h.subs[id] = ch
	h.mu.Unlock()
	cancel := func() {
		h.mu.Lock()
		sub, ok := h.subs[id]
		if ok {
			delete(h.subs, id)
		}
		h.mu.Unlock()
		if ok {
			close(sub)
		}
	}
	return ch, cancel
}

func (h *HITLBroker) broadcast(event HITLEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

// emitRequested forwards a newly registered request to the decision sink.
func (h *HITLBroker) emitRequested(ctx context.Context, req *PermissionRequest) {
	if h.decisions == nil || req == nil {
		return
	}
	h.decisions.HITLRequested(ctx, fwtelemetry.HITLRequest{
		RequestID:     req.ID,
		Action:        req.Permission.Action,
		Justification: req.Justification,
		RequiresHITL:  true,
	})
}

// emitResolved forwards a resolution to the decision sink. A nil decision
// means the request expired without a human decision.
func (h *HITLBroker) emitResolved(ctx context.Context, req *PermissionRequest, decision *PermissionDecision) {
	if h.decisions == nil || req == nil {
		return
	}
	if decision == nil {
		h.decisions.HITLResolved(ctx, fwtelemetry.HITLResolution{
			RequestID: req.ID,
			Outcome:   "expired",
		})
		return
	}
	outcome := "denied"
	approvedBy := decision.ApprovedBy
	if decision.Approved {
		outcome = "approved"
	}
	h.decisions.HITLResolved(ctx, fwtelemetry.HITLResolution{
		RequestID:  req.ID,
		Outcome:    outcome,
		ApprovedBy: approvedBy,
		Reason:     decision.Reason,
	})
}

// RequestPermission registers a request and waits for approval when possible.
func (h *HITLBroker) RequestPermission(ctx context.Context, req PermissionRequest) (*PermissionGrant, error) {
	if req.Permission.Action == "" {
		return nil, errors.New("permission request missing action")
	}
	req.ID = fmt.Sprintf("hitl-%d", h.clock().UnixNano())
	req.RequestedAt = h.clock()
	req.State = "pending"

	if h.AutoApprove {
		return &PermissionGrant{
			ID:          req.ID + ":auto-approve",
			Permission:  req.Permission,
			Scope:       req.Scope,
			ApprovedBy:  "auto-approve",
			GrantedAt:   h.clock(),
			Description: req.Justification,
		}, nil
	}

	waitCh := make(chan PermissionDecision, 1)

	h.mu.Lock()
	h.requests[req.ID] = &req
	h.waiters[req.ID] = waitCh
	h.mu.Unlock()
	h.emitRequested(ctx, &req)
	h.broadcast(HITLEvent{Type: HITLEventRequested, Request: &req})
	timeout := h.timeout
	if req.Timeout > 0 {
		timeout = req.Timeout
	}
	timeoutBehavior := req.TimeoutBehavior
	if timeoutBehavior == "" {
		timeoutBehavior = HITLTimeoutBehaviorFail
	}

	select {
	case decision := <-waitCh:
		deleteFn := func() {
			h.mu.Lock()
			delete(h.requests, req.ID)
			delete(h.waiters, req.ID)
			h.mu.Unlock()
		}
		defer deleteFn()
		h.emitResolved(ctx, &req, &decision)
		if !decision.Approved {
			h.broadcast(HITLEvent{Type: HITLEventResolved, Request: &req, Decision: &decision})
			return nil, fmt.Errorf("permission denied: %s", decision.Reason)
		}
		h.broadcast(HITLEvent{Type: HITLEventResolved, Request: &req, Decision: &decision})
		return &PermissionGrant{
			ID:          decision.RequestID,
			Permission:  req.Permission,
			Scope:       decision.Scope,
			ApprovedBy:  decision.ApprovedBy,
			Conditions:  decision.Conditions,
			GrantedAt:   h.clock(),
			ExpiresAt:   decision.ExpiresAt,
			Description: req.Justification,
		}, nil
	case <-ctx.Done():
		h.mu.Lock()
		delete(h.requests, req.ID)
		delete(h.waiters, req.ID)
		h.mu.Unlock()
		h.emitResolved(ctx, &req, nil)
		h.broadcast(HITLEvent{Type: HITLEventExpired, Request: &req, Error: ctx.Err().Error()})
		return nil, ctx.Err()
	case <-time.After(timeout):
		h.mu.Lock()
		delete(h.requests, req.ID)
		delete(h.waiters, req.ID)
		h.mu.Unlock()
		h.emitResolved(ctx, &req, nil)
		h.broadcast(HITLEvent{Type: HITLEventExpired, Request: &req, Error: "timed out"})
		if timeoutBehavior == HITLTimeoutBehaviorSkip {
			return &PermissionGrant{
				ID:          req.ID + ":timeout-skip",
				Permission:  req.Permission,
				Scope:       req.Scope,
				ApprovedBy:  "timeout-skip",
				Conditions:  map[string]string{"timeout": "true", "timeout_behavior": string(timeoutBehavior)},
				GrantedAt:   h.clock(),
				Description: req.Justification,
			}, nil
		}
		return nil, fmt.Errorf("permission request %s timed out", req.Permission.Action)
	}
}

// SubmitAsync registers a request without blocking.
func (h *HITLBroker) SubmitAsync(ctx context.Context, req PermissionRequest) (string, error) {
	req.ID = fmt.Sprintf("hitl-%d", h.clock().UnixNano())
	req.RequestedAt = h.clock()
	req.State = "pending"
	h.mu.Lock()
	if _, exists := h.requests[req.ID]; exists {
		h.mu.Unlock()
		return "", fmt.Errorf("request %s already registered", req.ID)
	}
	h.requests[req.ID] = &req
	h.mu.Unlock()
	h.emitRequested(ctx, &req)
	h.broadcast(HITLEvent{Type: HITLEventRequested, Request: &req})
	return req.ID, nil
}

// Approve asynchronously approves a request.
func (h *HITLBroker) Approve(decision PermissionDecision) error {
	return h.resolve(decision.RequestID, true, decision)
}

// Deny rejects a request.
func (h *HITLBroker) Deny(requestID, reason string) error {
	return h.resolve(requestID, false, PermissionDecision{
		RequestID: requestID,
		Approved:  false,
		Reason:    reason,
	})
}

// resolve is the single resolution path for Approve/Deny. It is the sole
// ownershp point for a request's waiter channel: the waiter is removed from
// the map under lock, so a duplicate resolution can never double-send (or
// send on a closed channel, which would panic while holding the broker's
// mutex). The decision sink is notified after the lock is released (NFR-3).
func (h *HITLBroker) resolve(requestID string, approved bool, decision PermissionDecision) error {
	h.mu.Lock()
	req, ok := h.requests[requestID]
	if !ok {
		h.mu.Unlock()
		return fmt.Errorf("request %s not found", requestID)
	}
	if req.State != "pending" {
		h.mu.Unlock()
		return fmt.Errorf("request %s already %s", requestID, req.State)
	}
	req.State = "resolved"
	if approved {
		req.State = "approved"
	} else {
		req.State = "denied"
	}
	if decision.Scope == "" && approved {
		decision.Scope = req.Scope
	}
	if approved && decision.ExpiresAt.IsZero() && decision.Scope == policy.GrantScopeOneTime {
		decision.ExpiresAt = h.clock().Add(time.Minute)
	}
	waiter, hasWaiter := h.waiters[requestID]
	delete(h.waiters, requestID)
	reqCopy := *req
	decisionCopy := decision
	h.mu.Unlock()

	if hasWaiter {
		// The waiter was popped under lock: this goroutine is its only
		// deliverer and the channel is never closed, so this send is safe.
		waiter <- decisionCopy
		// The blocking select in RequestPermission consumes the decision
		// and emits the resolution telemetry from the request's own
		// context; this path does not emit to avoid duplicate records.
		return nil
	}
	// Async-only resolution (SubmitAsync): no blocking waiter forwards it
	// to RequestPermission's select, so this is the sole emission point
	// for the resolution event. The request record is consumed here.
	h.mu.Lock()
	delete(h.requests, requestID)
	h.mu.Unlock()
	h.emitResolved(context.Background(), &reqCopy, &decisionCopy)
	go h.broadcast(HITLEvent{Type: HITLEventResolved, Request: &reqCopy, Decision: &decisionCopy})
	return nil
}

// PendingRequests returns the outstanding approvals.
func (h *HITLBroker) PendingRequests() []*PermissionRequest {
	h.mu.Lock()
	defer h.mu.Unlock()
	var pending []*PermissionRequest
	for _, req := range h.requests {
		if req.State == "pending" {
			pending = append(pending, req)
		}
	}
	return pending
}

// GrantManual creates a permission grant without the async flow.
func GrantManual(permission permissions.PermissionDescriptor, approvedBy string, scope policy.GrantScope, duration time.Duration) *PermissionGrant {
	grant := &PermissionGrant{
		ID:         fmt.Sprintf("manual-%d", time.Now().UnixNano()),
		Permission: permission,
		Scope:      scope,
		ApprovedBy: approvedBy,
		GrantedAt:  time.Now().UTC(),
	}
	if duration > 0 {
		grant.ExpiresAt = time.Now().Add(duration)
	}
	return grant
}
