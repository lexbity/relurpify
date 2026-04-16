package guidance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type GuidanceBroker struct {
	timeout         time.Duration
	defaultBehavior GuidanceTimeoutBehavior
	mu              sync.Mutex
	requests        map[string]*GuidanceRequest
	waiters         map[string]chan GuidanceDecision
	subs            map[int]chan GuidanceEvent
	subSeq          int
	deferralPlan    *DeferralPlan
	clock           func() time.Time
}

func NewGuidanceBroker(timeout time.Duration) *GuidanceBroker {
	if timeout == 0 {
		timeout = 3 * time.Minute
	}
	return &GuidanceBroker{
		timeout:         timeout,
		defaultBehavior: GuidanceTimeoutUseDefault,
		requests:        make(map[string]*GuidanceRequest),
		waiters:         make(map[string]chan GuidanceDecision),
		subs:            make(map[int]chan GuidanceEvent),
		clock:           func() time.Time { return time.Now().UTC() },
	}
}

func (g *GuidanceBroker) Request(ctx context.Context, req GuidanceRequest) (*GuidanceDecision, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}
	req = g.prepareRequest(req)
	waitCh := make(chan GuidanceDecision, 1)
	storedReq := cloneRequestPtr(&req)

	g.mu.Lock()
	g.requests[req.ID] = storedReq
	g.waiters[req.ID] = waitCh
	g.mu.Unlock()
	g.broadcast(GuidanceEvent{Type: GuidanceEventRequested, Request: cloneRequestPtr(&req)})

	timer := time.NewTimer(g.effectiveTimeout(req))
	defer timer.Stop()

	select {
	case decision := <-waitCh:
		g.cleanupRequest(req.ID)
		return &decision, nil
	case <-ctx.Done():
		g.cleanupRequest(req.ID)
		req.State = GuidanceStateExpired
		g.broadcast(GuidanceEvent{Type: GuidanceEventExpired, Request: cloneRequestPtr(&req), Error: ctx.Err().Error()})
		return nil, ctx.Err()
	case <-timer.C:
		return g.handleTimeout(req)
	}
}

func (g *GuidanceBroker) SubmitAsync(req GuidanceRequest) (string, error) {
	if err := validateRequest(req); err != nil {
		return "", err
	}
	req = g.prepareRequest(req)
	storedReq := cloneRequestPtr(&req)
	g.mu.Lock()
	if _, exists := g.requests[req.ID]; exists {
		g.mu.Unlock()
		return "", fmt.Errorf("guidance request %s already registered", req.ID)
	}
	g.requests[req.ID] = storedReq
	g.waiters[req.ID] = make(chan GuidanceDecision, 1)
	g.mu.Unlock()
	g.broadcast(GuidanceEvent{Type: GuidanceEventRequested, Request: cloneRequestPtr(&req)})
	return req.ID, nil
}

func (g *GuidanceBroker) Resolve(decision GuidanceDecision) error {
	if g == nil {
		return errors.New("guidance broker unavailable")
	}
	if decision.RequestID == "" {
		return errors.New("guidance decision missing request id")
	}
	g.mu.Lock()
	req, ok := g.requests[decision.RequestID]
	if !ok {
		g.mu.Unlock()
		return fmt.Errorf("guidance request %s not found", decision.RequestID)
	}
	if decision.DecidedAt.IsZero() {
		decision.DecidedAt = g.clock()
	}
	if decision.DecidedBy == "" {
		decision.DecidedBy = "user"
	}
	if decision.ChoiceID == "" && decision.Freetext == "" {
		g.mu.Unlock()
		return errors.New("guidance decision missing choice or freetext")
	}
	reqCopy := cloneRequestPtr(req)
	reqCopy.State = GuidanceStateResolved
	waiter := g.waiters[decision.RequestID]
	delete(g.requests, decision.RequestID)
	delete(g.waiters, decision.RequestID)
	g.mu.Unlock()

	if waiter != nil {
		waiter <- decision
		close(waiter)
	}
	g.broadcast(GuidanceEvent{Type: GuidanceEventResolved, Request: reqCopy, Decision: cloneDecisionPtr(&decision)})
	return nil
}

// EmitResolution broadcasts a resolved guidance event for an observation that
// was resolved outside the request/resolve request lifecycle.
func (g *GuidanceBroker) EmitResolution(observationID, resolvedBy string) {
	if g == nil {
		return
	}
	observationID = strings.TrimSpace(observationID)
	if observationID == "" {
		return
	}
	if strings.TrimSpace(resolvedBy) == "" {
		resolvedBy = "external"
	}
	g.broadcast(GuidanceEvent{
		Type: GuidanceEventResolved,
		Request: &GuidanceRequest{
			ID:    observationID,
			State: GuidanceStateResolved,
		},
		Decision: &GuidanceDecision{
			RequestID: observationID,
			DecidedBy: resolvedBy,
			DecidedAt: g.clock(),
		},
	})
}

func (g *GuidanceBroker) Subscribe(buffer int) (<-chan GuidanceEvent, func()) {
	if g == nil {
		ch := make(chan GuidanceEvent)
		close(ch)
		return ch, func() {}
	}
	if buffer <= 0 {
		buffer = 16
	}
	ch := make(chan GuidanceEvent, buffer)
	g.mu.Lock()
	id := g.subSeq
	g.subSeq++
	g.subs[id] = ch
	g.mu.Unlock()
	cancel := func() {
		g.mu.Lock()
		sub, ok := g.subs[id]
		if ok {
			delete(g.subs, id)
		}
		g.mu.Unlock()
		if ok {
			close(sub)
		}
	}
	return ch, cancel
}

func (g *GuidanceBroker) PendingRequests() []*GuidanceRequest {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]*GuidanceRequest, 0, len(g.requests))
	for _, req := range g.requests {
		if req == nil || req.State != GuidanceStatePending {
			continue
		}
		out = append(out, cloneRequestPtr(req))
	}
	return out
}

func (g *GuidanceBroker) SetDeferralPlan(dp *DeferralPlan) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.deferralPlan = dp
}

func (g *GuidanceBroker) AddObservation(obs EngineeringObservation) {
	if g == nil {
		return
	}
	g.mu.Lock()
	dp := g.deferralPlan
	clock := g.clock
	g.mu.Unlock()
	if dp == nil {
		return
	}
	if obs.CreatedAt.IsZero() {
		obs.CreatedAt = clock()
	}
	dp.AddObservation(obs)
}

func (g *GuidanceBroker) prepareRequest(req GuidanceRequest) GuidanceRequest {
	req.ID = fmt.Sprintf("guidance-%d", g.clock().UnixNano())
	req.RequestedAt = g.clock()
	req.State = GuidanceStatePending
	if req.TimeoutBehavior == "" {
		req.TimeoutBehavior = g.defaultBehavior
	}
	return req
}

func (g *GuidanceBroker) effectiveTimeout(req GuidanceRequest) time.Duration {
	if req.Timeout > 0 {
		return req.Timeout
	}
	return g.timeout
}

func (g *GuidanceBroker) handleTimeout(req GuidanceRequest) (*GuidanceDecision, error) {
	defaultChoice, ok := defaultChoice(req)
	behavior := req.TimeoutBehavior
	if behavior == "" {
		behavior = g.defaultBehavior
	}
	g.cleanupRequest(req.ID)

	switch behavior {
	case GuidanceTimeoutFail:
		req.State = GuidanceStateExpired
		g.broadcast(GuidanceEvent{Type: GuidanceEventExpired, Request: cloneRequestPtr(&req), Error: "timed out"})
		return nil, fmt.Errorf("guidance request %s timed out", req.ID)
	case GuidanceTimeoutDefer:
		if !ok {
			return nil, fmt.Errorf("guidance request %s has no default choice", req.ID)
		}
		decision := GuidanceDecision{
			RequestID: req.ID,
			ChoiceID:  defaultChoice.ID,
			DecidedBy: "deferred",
			DecidedAt: g.clock(),
		}
		req.State = GuidanceStateDeferred
		g.recordDeferredObservation(req)
		g.broadcast(GuidanceEvent{Type: GuidanceEventDeferred, Request: cloneRequestPtr(&req), Decision: cloneDecisionPtr(&decision)})
		return &decision, nil
	case GuidanceTimeoutUseDefault:
		fallthrough
	default:
		if !ok {
			return nil, fmt.Errorf("guidance request %s has no default choice", req.ID)
		}
		decision := GuidanceDecision{
			RequestID: req.ID,
			ChoiceID:  defaultChoice.ID,
			DecidedBy: "timeout-default",
			DecidedAt: g.clock(),
		}
		req.State = GuidanceStateExpired
		g.broadcast(GuidanceEvent{Type: GuidanceEventExpired, Request: cloneRequestPtr(&req), Decision: cloneDecisionPtr(&decision), Error: "timed out"})
		return &decision, nil
	}
}

func (g *GuidanceBroker) recordDeferredObservation(req GuidanceRequest) {
	g.mu.Lock()
	dp := g.deferralPlan
	g.mu.Unlock()
	if dp == nil {
		return
	}
	dp.AddObservation(EngineeringObservation{
		ID:           req.ID,
		Source:       req.ID,
		GuidanceKind: req.Kind,
		Title:        req.Title,
		Description:  req.Description,
		Evidence:     cloneContext(req.Context),
		CreatedAt:    g.clock(),
	})
}

func (g *GuidanceBroker) cleanupRequest(id string) {
	if g == nil || id == "" {
		return
	}
	g.mu.Lock()
	delete(g.requests, id)
	delete(g.waiters, id)
	g.mu.Unlock()
}

func (g *GuidanceBroker) broadcast(event GuidanceEvent) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, ch := range g.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func validateRequest(req GuidanceRequest) error {
	if req.Kind == "" {
		return errors.New("guidance request missing kind")
	}
	if req.Title == "" {
		return errors.New("guidance request missing title")
	}
	if len(req.Choices) == 0 {
		return errors.New("guidance request must include choices")
	}
	defaults := 0
	for _, choice := range req.Choices {
		if choice.ID == "" {
			return errors.New("guidance request choice missing id")
		}
		if choice.Label == "" {
			return errors.New("guidance request choice missing label")
		}
		if choice.IsDefault {
			defaults++
		}
	}
	if defaults > 1 {
		return errors.New("guidance request has multiple default choices")
	}
	return nil
}

func defaultChoice(req GuidanceRequest) (GuidanceChoice, bool) {
	for _, choice := range req.Choices {
		if choice.IsDefault {
			return choice, true
		}
	}
	return GuidanceChoice{}, false
}

func cloneRequestPtr(req *GuidanceRequest) *GuidanceRequest {
	if req == nil {
		return nil
	}
	out := *req
	if len(req.Choices) > 0 {
		out.Choices = append([]GuidanceChoice(nil), req.Choices...)
	}
	out.Context = cloneContext(req.Context)
	return &out
}

func cloneDecisionPtr(decision *GuidanceDecision) *GuidanceDecision {
	if decision == nil {
		return nil
	}
	out := *decision
	return &out
}

func cloneContext(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
