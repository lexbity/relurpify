package testhelper

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	euclopolicy "codeburg.org/lexbit/relurpify/named/euclo/policy"
)

// HITL broker test doubles (D13). The production authorization.HITLBroker no
// longer carries a auto-approval switch; these explicit fakes are the test-side
// replacement. Both satisfy euclopolicy.HITLBroker (RequestPermission) and
// authorization.HITLProvider.

// AutoApprovingBroker approves every permission request it receives.
type AutoApprovingBroker struct {
	mu       sync.Mutex
	requests []authorization.PermissionRequest
}

// NewAutoApprovingBroker creates an auto-approving broker.
func NewAutoApprovingBroker() *AutoApprovingBroker {
	return &AutoApprovingBroker{}
}

func (b *AutoApprovingBroker) RequestPermission(_ context.Context, req authorization.PermissionRequest) (*authorization.PermissionGrant, error) {
	if b == nil {
		return nil, fmt.Errorf("auto-approving broker is nil")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.requests = append(b.requests, req)
	now := time.Now().UTC()
	return &authorization.PermissionGrant{
		ID:          "auto-approving-" + strings.TrimSpace(req.Permission.Action),
		Permission:  req.Permission,
		Scope:       req.Scope,
		ApprovedBy:  "testhelper",
		GrantedAt:   now,
		ExpiresAt:   now.Add(5 * time.Minute),
		Description: req.Justification,
	}, nil
}

// Requests returns a copy of every request observed, in order.
func (b *AutoApprovingBroker) Requests() []authorization.PermissionRequest {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]authorization.PermissionRequest(nil), b.requests...)
}

// ScriptedBroker answers permission requests from a script keyed by permission
// action. An answer may be a grant (approval) or an error (denial);
// unauthored actions resolve by Default when set, else fail closed.
type ScriptedBroker struct {
	mu       sync.Mutex
	byAction map[string]*authorization.PermissionGrant
	errors   map[string]error
	Default  *authorization.PermissionGrant
	requests []authorization.PermissionRequest
}

// NewScriptedBroker builds an empty scripted broker (fail-closed until
// scripted).
func NewScriptedBroker() *ScriptedBroker {
	return &ScriptedBroker{
		byAction: make(map[string]*authorization.PermissionGrant),
		errors:   make(map[string]error),
	}
}

// Approve scripts an approval for the given permission action.
func (b *ScriptedBroker) Approve(action string) *ScriptedBroker {
	if b == nil {
		return b
	}
	now := time.Now().UTC()
	b.byAction[action] = &authorization.PermissionGrant{
		ID:         "scripted-" + action,
		ApprovedBy: "testhelper",
		GrantedAt:  now,
		ExpiresAt:  now.Add(5 * time.Minute),
	}
	return b
}

// Deny scripts a denial (as an error) for the given permission action.
func (b *ScriptedBroker) Deny(action, reason string) *ScriptedBroker {
	if b == nil {
		return b
	}
	b.errors[action] = fmt.Errorf("scripted denial: %s", reason)
	return b
}

func (b *ScriptedBroker) RequestPermission(_ context.Context, req authorization.PermissionRequest) (*authorization.PermissionGrant, error) {
	if b == nil {
		return nil, fmt.Errorf("scripted broker is nil")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.requests = append(b.requests, req)
	action := strings.TrimSpace(req.Permission.Action)
	if err, ok := b.errors[action]; ok {
		return nil, err
	}
	if grant, ok := b.byAction[action]; ok {
		return grant, nil
	}
	if b.Default != nil {
		return b.Default, nil
	}
	return nil, fmt.Errorf("scripted broker: no script for %q (fail-closed)", action)
}

// Requests returns a copy of every request observed, in order.
func (b *ScriptedBroker) Requests() []authorization.PermissionRequest {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]authorization.PermissionRequest(nil), b.requests...)
}

var (
	_ euclopolicy.HITLBroker = (*AutoApprovingBroker)(nil)
	_ euclopolicy.HITLBroker = (*ScriptedBroker)(nil)
)

// Interaction resolver test doubles (D12).

// PermissiveResolver answers every interaction frame immediately with the
// frame's default choice (or "abort" when the frame has none). It is the
// surface double for flows that must not block on a human.
type PermissiveResolver struct {
	mu       sync.Mutex
	resolved map[string]interaction.FrameResolution
	notified []string
}

// NewPermissiveResolver creates a permissive resolver.
func NewPermissiveResolver() *PermissiveResolver {
	return &PermissiveResolver{resolved: make(map[string]interaction.FrameResolution)}
}

func (r *PermissiveResolver) Resolve(_ context.Context, frame *interaction.InteractionFrame) (interaction.FrameResolution, error) {
	if r == nil || frame == nil || strings.TrimSpace(frame.ID) == "" {
		return interaction.FrameResolution{Status: interaction.ResolutionExpired}, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if resolution, ok := r.resolved[frame.ID]; ok {
		return resolution, nil
	}
	answer := strings.TrimSpace(frame.DefaultChoice)
	if answer == "" {
		answer = "abort"
	}
	resolution := interaction.FrameResolution{Status: interaction.ResolutionAnswered, Answer: answer}
	r.resolved[frame.ID] = resolution
	return resolution, nil
}

func (r *PermissiveResolver) Notify(_ context.Context, frame *interaction.InteractionFrame) error {
	if r == nil || frame == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notified = append(r.notified, strings.TrimSpace(frame.ID))
	return nil
}

// Notified reports the frame IDs delivered via Notify, sorted.
func (r *PermissiveResolver) Notified() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]string(nil), r.notified...)
	sort.Strings(out)
	return out
}

// ScriptedResolver answers pre-registered frame IDs with scripted resolutions;
// unknown frames resolve as Expired. It is the deterministic double for the
// exactly-once and failure-path resolver tests.
type ScriptedResolver struct {
	mu       sync.Mutex
	byFrame  map[string]interaction.FrameResolution
	notified []string
}

// NewScriptedResolver builds an empty scripted resolver.
func NewScriptedResolver() *ScriptedResolver {
	return &ScriptedResolver{
		byFrame: make(map[string]interaction.FrameResolution),
	}
}

// Answer scripts a resolution for the given frame ID.
func (r *ScriptedResolver) Answer(frameID string, resolution interaction.FrameResolution) *ScriptedResolver {
	if r != nil && resolution.Status != "" {
		r.byFrame[frameID] = resolution
	}
	return r
}

func (r *ScriptedResolver) Resolve(_ context.Context, frame *interaction.InteractionFrame) (interaction.FrameResolution, error) {
	if r == nil || frame == nil || strings.TrimSpace(frame.ID) == "" {
		return interaction.FrameResolution{Status: interaction.ResolutionExpired}, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if resolution, ok := r.byFrame[frame.ID]; ok {
		return resolution, nil
	}
	return interaction.FrameResolution{Status: interaction.ResolutionExpired}, nil
}

func (r *ScriptedResolver) Notify(_ context.Context, frame *interaction.InteractionFrame) error {
	if r == nil || frame == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notified = append(r.notified, strings.TrimSpace(frame.ID))
	return nil
}

// ExpiredClock is a mutable, injectable time source for resolver/deadline
// tests. Reading it returns the current value; Advance moves it forward.
type ExpiredClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewExpiredClock starts the clock at the given instant (zero uses a fixed
// reference time).
func NewExpiredClock(at time.Time) *ExpiredClock {
	if at.IsZero() {
		at = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	}
	return &ExpiredClock{now: at}
}

// Now returns the current clock reading.
func (c *ExpiredClock) Now() time.Time {
	if c == nil {
		return time.Now()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d.
func (c *ExpiredClock) Advance(d time.Duration) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// DeadlineHelper returns the clock as a func() time.Time for resolvers that
// accept a time source.
func (c *ExpiredClock) DeadlineHelper() func() time.Time {
	return c.Now
}
