package learning

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type EventType string

const (
	EventRequested EventType = "requested"
	EventResolved  EventType = "resolved"
	EventDeferred  EventType = "deferred"
	EventExpired   EventType = "expired"
)

type Event struct {
	Type        EventType
	Interaction *Interaction
	Error       string
}

type Broker struct {
	timeout         time.Duration
	defaultBehavior TimeoutBehavior
	mu              sync.Mutex
	requests        map[string]*Interaction
	waiters         map[string]chan *Interaction
	subs            map[int]chan Event
	subSeq          int
	clock           func() time.Time
}

func NewBroker(timeout time.Duration) *Broker {
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	return &Broker{
		timeout:         timeout,
		defaultBehavior: TimeoutUseDefault,
		requests:        make(map[string]*Interaction),
		waiters:         make(map[string]chan *Interaction),
		subs:            make(map[int]chan Event),
		clock:           func() time.Time { return time.Now().UTC() },
	}
}

func (b *Broker) Request(ctx context.Context, interaction Interaction) (*Interaction, error) {
	if err := validateInteraction(interaction); err != nil {
		return nil, err
	}
	waitCh := make(chan *Interaction, 1)
	copy := cloneInteraction(&interaction)

	b.mu.Lock()
	b.requests[interaction.ID] = copy
	b.waiters[interaction.ID] = waitCh
	b.mu.Unlock()
	b.broadcast(Event{Type: EventRequested, Interaction: cloneInteraction(copy)})

	timer := time.NewTimer(b.timeout)
	defer timer.Stop()

	select {
	case resolved := <-waitCh:
		b.cleanup(interaction.ID)
		return resolved, nil
	case <-ctx.Done():
		b.cleanup(interaction.ID)
		copy.Status = StatusExpired
		copy.UpdatedAt = b.now()
		b.broadcast(Event{Type: EventExpired, Interaction: cloneInteraction(copy), Error: ctx.Err().Error()})
		return nil, ctx.Err()
	case <-timer.C:
		resolved, err := b.handleTimeout(copy)
		if err != nil {
			return nil, err
		}
		return resolved, nil
	}
}

func (b *Broker) SubmitAsync(interaction Interaction) error {
	if err := validateInteraction(interaction); err != nil {
		return err
	}
	copy := cloneInteraction(&interaction)
	b.mu.Lock()
	if _, exists := b.requests[interaction.ID]; exists {
		b.mu.Unlock()
		return fmt.Errorf("learning interaction %s already registered", interaction.ID)
	}
	b.requests[interaction.ID] = copy
	b.waiters[interaction.ID] = make(chan *Interaction, 1)
	b.mu.Unlock()
	b.broadcast(Event{Type: EventRequested, Interaction: cloneInteraction(copy)})
	return nil
}

func (b *Broker) Resolve(interaction Interaction) error {
	if b == nil {
		return errors.New("learning broker unavailable")
	}
	if stringsTrim(interaction.ID) == "" {
		return errors.New("learning interaction id required")
	}
	if interaction.Status != StatusResolved && interaction.Status != StatusDeferred && interaction.Status != StatusExpired {
		return fmt.Errorf("learning interaction %s must be resolved, deferred, or expired", interaction.ID)
	}
	b.mu.Lock()
	_, ok := b.requests[interaction.ID]
	waiter := b.waiters[interaction.ID]
	delete(b.requests, interaction.ID)
	delete(b.waiters, interaction.ID)
	b.mu.Unlock()
	if !ok {
		return fmt.Errorf("learning interaction %s not found", interaction.ID)
	}
	copy := cloneInteraction(&interaction)
	if waiter != nil {
		waiter <- copy
		close(waiter)
	}
	eventType := EventResolved
	switch interaction.Status {
	case StatusDeferred:
		eventType = EventDeferred
	case StatusExpired:
		eventType = EventExpired
	}
	b.broadcast(Event{Type: eventType, Interaction: cloneInteraction(copy)})
	return nil
}

func (b *Broker) Subscribe(buffer int) (<-chan Event, func()) {
	if b == nil {
		ch := make(chan Event)
		close(ch)
		return ch, func() {}
	}
	if buffer <= 0 {
		buffer = 16
	}
	ch := make(chan Event, buffer)
	b.mu.Lock()
	id := b.subSeq
	b.subSeq++
	b.subs[id] = ch
	b.mu.Unlock()
	cancel := func() {
		b.mu.Lock()
		sub, ok := b.subs[id]
		if ok {
			delete(b.subs, id)
		}
		b.mu.Unlock()
		if ok {
			close(sub)
		}
	}
	return ch, cancel
}

func (b *Broker) PendingInteractions() []Interaction {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Interaction, 0, len(b.requests))
	for _, interaction := range b.requests {
		if interaction == nil || interaction.Status != StatusPending {
			continue
		}
		out = append(out, *cloneInteraction(interaction))
	}
	return out
}

func (b *Broker) handleTimeout(interaction *Interaction) (*Interaction, error) {
	b.cleanup(interaction.ID)
	behavior := interaction.TimeoutBehavior
	if behavior == "" {
		behavior = b.defaultBehavior
	}
	copy := cloneInteraction(interaction)
	copy.UpdatedAt = b.now()
	switch behavior {
	case TimeoutExpire:
		copy.Status = StatusExpired
		b.broadcast(Event{Type: EventExpired, Interaction: cloneInteraction(copy), Error: "timed out"})
		return copy, nil
	case TimeoutDefer:
		copy.Status = StatusDeferred
		copy.Resolution = &Resolution{Kind: ResolutionDefer, ResolvedBy: "timeout-defer", ResolvedAt: copy.UpdatedAt}
		b.broadcast(Event{Type: EventDeferred, Interaction: cloneInteraction(copy)})
		return copy, nil
	case TimeoutUseDefault:
		fallthrough
	default:
		copy.Status = StatusResolved
		copy.Resolution = &Resolution{
			Kind:       resolutionKindForChoice(copy.DefaultChoice),
			ChoiceID:   copy.DefaultChoice,
			ResolvedBy: "timeout-default",
			ResolvedAt: copy.UpdatedAt,
		}
		b.broadcast(Event{Type: EventResolved, Interaction: cloneInteraction(copy)})
		return copy, nil
	}
}

func (b *Broker) cleanup(id string) {
	if b == nil || id == "" {
		return
	}
	b.mu.Lock()
	delete(b.requests, id)
	delete(b.waiters, id)
	b.mu.Unlock()
}

func (b *Broker) broadcast(event Event) {
	if b == nil {
		return
	}
	b.mu.Lock()
	subs := make([]chan Event, 0, len(b.subs))
	for _, sub := range b.subs {
		subs = append(subs, sub)
	}
	b.mu.Unlock()
	for _, sub := range subs {
		select {
		case sub <- event:
		default:
		}
	}
}

func (b *Broker) now() time.Time {
	if b.clock != nil {
		return b.clock().UTC()
	}
	return time.Now().UTC()
}

func validateInteraction(interaction Interaction) error {
	if stringsTrim(interaction.ID) == "" {
		return errors.New("learning interaction id required")
	}
	if interaction.Status == "" {
		return errors.New("learning interaction status required")
	}
	return nil
}

func resolutionKindForChoice(choiceID string) ResolutionKind {
	switch ResolutionKind(choiceID) {
	case ResolutionReject, ResolutionRefine, ResolutionDefer:
		return ResolutionKind(choiceID)
	default:
		return ResolutionConfirm
	}
}

func cloneInteraction(interaction *Interaction) *Interaction {
	if interaction == nil {
		return nil
	}
	copy := *interaction
	copy.Evidence = append([]EvidenceRef(nil), interaction.Evidence...)
	copy.Choices = append([]Choice(nil), interaction.Choices...)
	if interaction.Resolution != nil {
		resolution := *interaction.Resolution
		if interaction.Resolution.RefinedPayload != nil {
			resolution.RefinedPayload = cloneMap(interaction.Resolution.RefinedPayload)
		}
		copy.Resolution = &resolution
	}
	return &copy
}

func stringsTrim(value string) string {
	return strings.TrimSpace(value)
}
